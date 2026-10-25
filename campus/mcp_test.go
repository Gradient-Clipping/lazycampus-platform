package campus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mcpAuthTransport struct{ key string }

func (t mcpAuthTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+t.key)
	return http.DefaultTransport.RoundTrip(r)
}

func TestMCPFullCatalogHasStrictSchemasAndWriteAnnotations(t *testing.T) {
	a, user, _ := testApp(t)
	token, key := testKey(t, a, user)
	scopes := []string{}
	for _, e := range Catalog {
		scopes = append(scopes, e.Scope)
	}
	require.NoError(t, a.DB.Model(&token).Update("scopes", strings.Join(scopes, " ")).Error)
	session := connectMCP(t, a, key)
	list, err := session.ListTools(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, list.Tools, 24)
	seen := map[string]bool{}
	for _, tool := range list.Tools {
		assert.False(t, seen[tool.Name], "duplicate tool")
		seen[tool.Name] = true
		assert.NotEmpty(t, tool.Description)
		assert.Contains(t, tool.Description, "Required scope:")
		assert.Equal(t, -1, strings.IndexFunc(tool.Description+tool.Title, func(r rune) bool { return unicode.Is(unicode.Han, r) }), tool.Name)
		raw, err := json.Marshal(tool.InputSchema)
		require.NoError(t, err)
		assert.Contains(t, string(raw), `"additionalProperties":false`)
		require.NotNil(t, tool.Annotations)
		if tool.Name == "issue_my_certificate" {
			assert.False(t, tool.Annotations.ReadOnlyHint)
			assert.False(t, tool.Annotations.IdempotentHint)
			assert.Contains(t, string(raw), `"confirm"`)
			assert.Contains(t, string(raw), `"const":true`)
		} else {
			assert.True(t, tool.Annotations.ReadOnlyHint)
		}
	}
	for _, e := range Catalog {
		_, supported := mcpToolNames[e.Path]
		assert.Equal(t, reference.Endpoints[e.Path].ContentType == "application/json", supported, e.Path)
	}
}

func connectMCP(t *testing.T, a *App, key string) *mcp.ClientSession {
	t.Helper()
	server := httptest.NewServer(a.Router())
	t.Cleanup(server.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "integration-test", Version: "1.0.0"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.URL + "/mcp", HTTPClient: &http.Client{Transport: mcpAuthTransport{key}}, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestMCPToolsUseCurrentPermissionsAndSharedGateway(t *testing.T) {
	a, user, _ := testApp(t)
	token, key := testKey(t, a, user)
	require.NoError(t, a.DB.Model(&token).Updates(map[string]any{"scopes": "timetable:read laundry:read", "quota": 1}).Error)
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Empty(t, r.Header.Get("Authorization"))
		assert.NotEmpty(t, r.Header.Get("X-Platform-Signature"))
		assert.Equal(t, "/internal/platform/v1/utilities/laundry/devices", r.URL.Path)
		assert.Empty(t, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":{"binding":null,"devices":[]},"meta":{"cached":true,"fetchedAt":"2026-10-25T00:00:00Z"}}`)
	}))
	defer upstream.Close()
	a.Config.CampusURL = upstream.URL
	session := connectMCP(t, a, key)
	ctx := context.Background()
	list, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	names := []string{}
	for _, tool := range list.Tools {
		names = append(names, tool.Name)
	}
	assert.ElementsMatch(t, []string{"get_my_timetable", "get_my_schedule", "get_bound_dorm_laundry"}, names)
	resources, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "lazycampus://guide"})
	require.NoError(t, err)
	assert.Contains(t, resources.Contents[0].Text, "untrusted data")
	var stored Token
	require.NoError(t, a.DB.First(&stored, token.ID).Error)
	assert.Zero(t, stored.UsedQuota, "handshake and discovery are not data calls")
	bad, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_bound_dorm_laundry", Arguments: map[string]any{"campus": "foreign"}})
	require.NoError(t, err)
	assert.True(t, bad.IsError)
	assert.Zero(t, calls)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_bound_dorm_laundry", Arguments: map[string]any{}})
	require.NoError(t, err)
	require.False(t, result.IsError, "%+v", result.Content)
	raw, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"httpStatus":200`)
	assert.Contains(t, string(raw), `"cached":true`)
	assert.Contains(t, string(raw), `"binding":null`)
	assert.NotContains(t, string(raw), key)
	assert.Equal(t, 1, calls)
	// The REST route shares the same remaining total quota.
	assert.Equal(t, 429, apiCall(a, key, "/v1/teaching/timetable").Code)
	limited, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_my_timetable", Arguments: map[string]any{}})
	require.NoError(t, err)
	assert.True(t, limited.IsError)
	raw, err = json.Marshal(limited.StructuredContent)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"httpStatus":429`)
	assert.Contains(t, string(raw), `QUOTA_EXHAUSTED`)
	var log Log
	require.NoError(t, a.DB.Where("token_id = ? AND admitted = ?", token.ID, true).First(&log).Error)
	assert.Equal(t, "/utilities/laundry/devices", log.Endpoint)
	assert.NotEmpty(t, log.RequestID)
	// Tool lists update on the next request, without trusting a prior handshake.
	require.NoError(t, a.DB.Model(&token).Update("scopes", "timetable:read").Error)
	list, err = session.ListTools(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, list.Tools, 2)
	require.NoError(t, a.DB.Model(&EndpointPolicy{}).Where("path = ?", "/teaching/schedule").Update("status", "maintenance").Error)
	list, err = session.ListTools(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, list.Tools, 1)
	_, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "get_bound_dorm_laundry", Arguments: map[string]any{}})
	assert.Error(t, err)
	require.NoError(t, a.DB.Model(&token).Update("suspended", true).Error)
	_, err = session.ListTools(ctx, nil)
	assert.Error(t, err)
}

func TestMCPCertificateConsentValidationAndJobResult(t *testing.T) {
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
		assert.NotContains(t, string(raw), "confirm")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		fmt.Fprint(w, `{"success":true,"data":{"jobId":"job-test"}}`)
	}))
	defer upstream.Close()
	a.Config.CampusURL = upstream.URL
	session := connectMCP(t, a, key)
	for _, args := range []map[string]any{
		{"id": "transcript"}, {"id": "transcript", "confirm": false},
		{"id": "transcript", "confirm": true, "selected": nil},
		{"id": "transcript", "confirm": true, "userId": user.ID + 1},
		{"id": "../foreign", "confirm": true},
	} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "issue_my_certificate", Arguments: args})
		require.NoError(t, err)
		assert.True(t, result.IsError, "%v", args)
	}
	assert.Zero(t, calls)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "issue_my_certificate", Arguments: map[string]any{"id": "transcript", "confirm": true, "selected": map[string]string{"language": "en"}}})
	require.NoError(t, err)
	require.False(t, result.IsError, "%+v", result.Content)
	raw, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"httpStatus":202`)
	assert.Contains(t, string(raw), `"jobId":"job-test"`)
	assert.Equal(t, 1, calls)
}

func TestMCPTransportRejectsInvalidOriginCredentialsAndQuery(t *testing.T) {
	a, user, _ := testApp(t)
	token, key := testKey(t, a, user)
	call := func(key, origin, query, method string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/mcp"+query, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		r.RemoteAddr = "198.51.100.222:8080"
		r.Header.Set("Authorization", "Bearer "+key)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		a.Router().ServeHTTP(w, r)
		return w
	}
	assert.Equal(t, 403, call(key, "https://evil.example", "", "POST").Code)
	assert.Equal(t, 400, call(key, "", "?key=secret", "POST").Code)
	assert.Equal(t, 401, call("bad", "", "", "POST").Code)
	assert.Equal(t, 405, call(key, "", "", "GET").Code)
	assert.Equal(t, 200, call(key, a.Config.Origin, "", "POST").Code)
	require.NoError(t, a.DB.Model(&token).Update("allowed_ips", "192.0.2.1").Error)
	assert.Equal(t, 403, call(key, "", "", "POST").Code)
}
