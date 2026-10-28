package campus

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func graphqlCall(a *App, key, query string, variables map[string]any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(map[string]any{"query": query, "variables": variables})
	r := httptest.NewRequest("POST", "/graphql", strings.NewReader(string(raw)))
	r.Header.Set("Authorization", "Bearer "+key)
	r.Header.Set("Content-Type", "application/json")
	r.RemoteAddr = "198.51.100.222:8080"
	w := httptest.NewRecorder()
	a.Router().ServeHTTP(w, r)
	return w
}

func TestGraphQLSchemaIntrospectionAndExamples(t *testing.T) {
	a := &App{}
	schema, err := a.graphqlSchema(&gin.Context{}, Catalog)
	require.NoError(t, err)
	assert.Len(t, schema.QueryType().Fields(), 24, "23 data queries plus apiGuide")
	assert.Len(t, schema.MutationType().Fields(), 1)
	for _, e := range Catalog {
		name, supported := mcpToolNames[e.Path]
		if !supported {
			continue
		}
		fields := schema.QueryType().Fields()
		if e.Method == "POST" {
			fields = schema.MutationType().Fields()
		}
		field := fields[graphqlFieldName(name)]
		require.NotNil(t, field, e.Path)
		assert.Contains(t, field.Description, e.Scope)
	}
	result := graphql.Do(graphql.Params{Schema: schema, RequestString: `{ __schema { queryType { name description fields { name args { name type { kind name ofType { name } } } } } mutationType { name } } apiGuide }`})
	require.Empty(t, result.Errors)
	raw, _ := json.Marshal(result)
	assert.Contains(t, string(raw), "getMyTimetable")
	assert.Contains(t, string(raw), "partial failures")
	// Validate the same operations shown on the connection guide without invoking upstream.
	for _, q := range []string{
		`query CampusOverview($semester: String) { getMyTimetable(semester: $semester) { data { semesters { id label } } meta { cached fetchedAt refreshing } requestId } getBoundDormLaundry { data { binding { campus name } devices { id name stateName } } requestId } }`,
		`mutation IssueCertificate($id: String!, $selected: JSON) { issueMyCertificate(id: $id, selected: $selected, confirm: true) { httpStatus data { jobId } requestId } }`,
	} {
		document, err := parser.Parse(parser.ParseParams{Source: q})
		require.NoError(t, err)
		assert.Empty(t, graphql.ValidateDocument(&schema, document, nil).Errors)
	}
}

func TestGraphQLSharedQuotaFieldSelectionAndPartialErrors(t *testing.T) {
	a, user, _ := testApp(t)
	token, key := testKey(t, a, user)
	require.NoError(t, a.DB.Model(&token).Updates(map[string]any{"scopes": "laundry:read", "quota": 2}).Error)
	require.NoError(t, a.DB.Model(&EndpointPolicy{}).Where("path = ?", "/utilities/laundry/devices").Update("cooldown", 0).Error)
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, "/internal/platform/v1/utilities/laundry/devices", r.URL.Path)
		assert.Empty(t, r.URL.RawQuery)
		assert.Empty(t, r.Header.Get("Authorization"))
		assert.NotEmpty(t, r.Header.Get("X-Platform-Signature"))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":{"binding":null,"devices":[]},"meta":{"cached":true,"fetchedAt":"2026-10-25T00:00:00Z"},"unused":"must not appear"}`)
	}))
	defer upstream.Close()
	a.Config.CampusURL = upstream.URL
	discovery := graphqlCall(a, key, `{ apiGuide __schema { queryType { name } } }`, nil)
	require.Equal(t, 200, discovery.Code, discovery.Body.String())
	var stored Token
	require.NoError(t, a.DB.First(&stored, token.ID).Error)
	assert.Zero(t, stored.UsedQuota)
	bad := graphqlCall(a, key, `{ getBoundDormLaundry(campus: "foreign") { success } }`, nil)
	assert.Equal(t, 400, bad.Code)
	assert.Zero(t, calls)
	response := graphqlCall(a, key, `{ laundry: getBoundDormLaundry { success data { binding { campus } devices { id } } meta { cached fetchedAt } httpStatus requestId quotaRemaining } another: getBoundDormLaundry { success requestId } denied: getMyTimetable { success } }`, nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"binding":null`)
	assert.Contains(t, response.Body.String(), `"cached":true`)
	assert.Contains(t, response.Body.String(), `"httpStatus":200`)
	assert.Contains(t, response.Body.String(), `"path":["denied"]`)
	assert.Contains(t, response.Body.String(), `SCOPE_REQUIRED`)
	assert.NotContains(t, response.Body.String(), "unused")
	assert.NotContains(t, response.Body.String(), key)
	assert.Equal(t, 2, calls)
	assert.Equal(t, 429, apiCall(a, key, "/v1/utilities/laundry/devices").Code)
	limited := graphqlCall(a, key, `{ getBoundDormLaundry { success } }`, nil)
	assert.Equal(t, 200, limited.Code)
	assert.Contains(t, limited.Body.String(), "QUOTA_EXHAUSTED")
	assert.Contains(t, limited.Body.String(), `"retryAfterSeconds":`)
	var logs []Log
	require.NoError(t, a.DB.Where("token_id = ?", token.ID).Find(&logs).Error)
	assert.GreaterOrEqual(t, len(logs), 3)
	seen := map[string]bool{}
	for _, log := range logs {
		assert.False(t, seen[log.RequestID])
		seen[log.RequestID] = true
	}
	require.NoError(t, a.DB.Model(&token).Update("suspended", true).Error)
	assert.Equal(t, 401, graphqlCall(a, key, `{ apiGuide }`, nil).Code)
}

func TestGraphQLLaundryPreservesNumericAndNullFloors(t *testing.T) {
	a, user, _ := testApp(t)
	token, key := testKey(t, a, user)
	require.NoError(t, a.DB.Model(&token).Update("scopes", "laundry:read").Error)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/internal/platform/v1/utilities/laundry/devices", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":{"devices":[{"floor":3},{"floor":null},{"floor":-1}]}}`)
	}))
	defer upstream.Close()
	a.Config.CampusURL = upstream.URL
	response := graphqlCall(a, key, `{ getBoundDormLaundry { success data { devices { floor } } } }`, nil)
	require.Equal(t, 200, response.Code)
	assert.JSONEq(t, `{"data":{"getBoundDormLaundry":{"success":true,"data":{"devices":[{"floor":3},{"floor":null},{"floor":-1}]}}}}`, response.Body.String())
}

func TestGraphQLVariablesMutationConsentAndLivePolicy(t *testing.T) {
	a, user, _ := testApp(t)
	token, key := testKey(t, a, user)
	require.NoError(t, a.DB.Model(&token).Update("scopes", "documents:issue").Error)
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/internal/platform/v1/campus-documents/certificates/transcript/issue", r.URL.Path)
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"selected":{"language":"en"}}`, string(raw))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		fmt.Fprint(w, `{"success":true,"data":{"jobId":"job-test"}}`)
	}))
	defer upstream.Close()
	a.Config.CampusURL = upstream.URL
	query := `mutation IssueCertificate($id: String!, $selected: JSON, $confirm: Boolean!) { issueMyCertificate(id: $id, selected: $selected, confirm: $confirm) { httpStatus data { jobId } requestId } }`
	for _, args := range []map[string]any{
		{"id": "transcript", "confirm": false}, {"id": "../foreign", "confirm": true},
		{"id": "transcript", "confirm": true, "selected": map[string]any{"language": 123}},
	} {
		response := graphqlCall(a, key, query, args)
		assert.Contains(t, response.Body.String(), `"errors":`)
	}
	assert.Zero(t, calls)
	args := map[string]any{"id": "transcript", "confirm": true, "selected": map[string]string{"language": "en"}}
	result := graphqlCall(a, key, query, args)
	require.Equal(t, 200, result.Code, result.Body.String())
	assert.NotContains(t, result.Body.String(), `"errors":`)
	assert.Contains(t, result.Body.String(), `"httpStatus":202`)
	assert.Contains(t, result.Body.String(), `"jobId":"job-test"`)
	assert.Equal(t, 1, calls)
	require.NoError(t, a.DB.Model(&EndpointPolicy{}).Where("path = ?", "/campus-documents/certificates/{id}/issue").Update("status", "maintenance").Error)
	result = graphqlCall(a, key, query, args)
	assert.Contains(t, result.Body.String(), "ENDPOINT_UNAVAILABLE")
	assert.Equal(t, 1, calls)
}

func TestGraphQLRejectsInvalidTransportAndExpensiveDocuments(t *testing.T) {
	a, user, _ := testApp(t)
	_, key := testKey(t, a, user)
	for _, query := range []string{
		`query {`, `{ doesNotExist }`, `subscription { apiGuide }`,
		`{ ...Cycle } fragment Cycle on Query { ...Cycle }`,
		`{ a:apiGuide b:apiGuide c:apiGuide d:apiGuide e:apiGuide f:apiGuide g:apiGuide h:apiGuide i:apiGuide j:apiGuide k:apiGuide }`,
		`{ __type(name:"Query") { ` + strings.Repeat("ofType { ", 17) + "name" + strings.Repeat(" }", 17) + ` } }`,
	} {
		assert.Equal(t, 400, graphqlCall(a, key, query, nil).Code, query)
	}
	for _, tc := range []struct {
		method, path, contentType, origin, body string
		status                                  int
	}{
		{"GET", "/graphql", "", "", "", 405},
		{"POST", "/graphql?key=secret", "application/json", "", `{}`, 400},
		{"POST", "/graphql", "application/json", "https://evil.example", `{}`, 403},
		{"POST", "/graphql", "text/plain", "", `{}`, 415},
		{"POST", "/graphql", "application/json", "", `[{"query":"{apiGuide}"}]`, 400},
		{"POST", "/graphql", "application/json", "", `{"query":"{apiGuide}"} {}`, 400},
		{"POST", "/graphql", "application/json", "", `{"query":"` + strings.Repeat(" ", 33*1024) + `{apiGuide}"}`, 400},
	} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		r.Header.Set("Authorization", "Bearer "+key)
		r.Header.Set("Content-Type", tc.contentType)
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		a.Router().ServeHTTP(w, r)
		assert.Equal(t, tc.status, w.Code, w.Body.String())
	}
	assert.Equal(t, 401, graphqlCall(a, "bad", `{ apiGuide }`, nil).Code)
}
