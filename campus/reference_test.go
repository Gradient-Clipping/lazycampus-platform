package campus

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReferenceMatchesPublicGateway(t *testing.T) {
	require.Len(t, reference.Endpoints, len(Catalog))
	for _, endpoint := range Catalog {
		t.Run(endpoint.Path, func(t *testing.T) {
			doc, ok := reference.Endpoints[endpoint.Path]
			require.True(t, ok)
			require.NotEmpty(t, doc.Summary)
			require.NotEmpty(t, doc.Notes)
			require.Len(t, doc.Parameters, len(endpoint.Parameters), "must only document public query parameters")
			for _, parameter := range endpoint.Parameters {
				detail, ok := doc.Parameters[parameter.Name]
				require.True(t, ok, parameter.Name)
				require.NotEmpty(t, detail.Description)
				require.NotEmpty(t, detail.Schema["type"])
				if parameter.Required {
					require.NotNil(t, detail.Example, "required request examples must be complete")
				}
			}
			for _, related := range doc.Related {
				_, ok := endpointFor(related)
				require.True(t, ok, related)
			}
			validateReferenceLinks(t, doc.ResponseSchema, nil)
			if doc.ContentType == "application/json" {
				require.NotNil(t, doc.ResponseExample)
				example := doc.ResponseExample.(map[string]any)
				assert.Equal(t, true, example["success"])
				assert.Contains(t, example, "data")
			} else {
				assert.Equal(t, "binary", doc.ResponseSchema["format"])
			}
		})
	}
}

func validateReferenceLinks(t *testing.T, value any, visiting []string) {
	t.Helper()
	switch node := value.(type) {
	case map[string]any:
		if link, ok := node["$ref"].(string); ok {
			assert.True(t, strings.HasPrefix(link, "#/components/schemas/"))
			require.NotContains(t, visiting, link, "schema expansion must not recurse forever")
			schema, ok := reference.Schemas[strings.TrimPrefix(link, "#/components/schemas/")]
			require.True(t, ok, link)
			validateReferenceLinks(t, schema, append(visiting, link))
			return
		}
		for _, child := range node {
			validateReferenceLinks(t, child, visiting)
		}
	case []any:
		for _, child := range node {
			validateReferenceLinks(t, child, visiting)
		}
	}
}

func TestOpenAPIExportsCurrentPolicyAndResponseContracts(t *testing.T) {
	a, _, cookie := testApp(t)
	require.NoError(t, a.DB.Model(&EndpointPolicy{}).Where("path = ?", "/teaching/grades").Updates(map[string]any{"scope": "timetable:read", "rate_limit": 7, "daily_quota": 777, "status": "maintenance", "message": "测试维护"}).Error)
	w := perform(a, "GET", "/openapi.json", "", "", "")
	require.Equal(t, 200, w.Code)
	var spec map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &spec))
	paths := spec["paths"].(map[string]any)
	assert.Len(t, paths, 18)
	grade := paths["/v1/teaching/grades"].(map[string]any)["get"].(map[string]any)
	assert.Equal(t, "timetable:read", grade["x-required-scope"])
	assert.Equal(t, "maintenance", grade["x-service-status"])
	assert.EqualValues(t, 7, grade["x-rate-limit-per-minute"])
	assert.Contains(t, grade["description"], "测试维护")
	for _, raw := range grade["parameters"].([]any) {
		parameter := raw.(map[string]any)
		if parameter["name"] == "pageSize" {
			schema := parameter["schema"].(map[string]any)
			assert.Equal(t, "integer", schema["type"])
			assert.EqualValues(t, 50, schema["default"])
			assert.EqualValues(t, 100, schema["maximum"])
		}
	}
	assert.Contains(t, grade["responses"], "404")
	assert.Contains(t, spec["components"].(map[string]any)["schemas"], "APIError")
	assert.NotContains(t, paths, "/api/tokens", "developer keys cannot manage accounts or applications")
	for _, path := range []string{"/v1/teaching/calendar/image", "/v1/teaching/notices/attachment"} {
		operation := paths[path].(map[string]any)["get"].(map[string]any)
		content := operation["responses"].(map[string]any)["200"].(map[string]any)["content"].(map[string]any)
		assert.NotContains(t, content, "application/json", "successful downloads are binary")
	}
	list := perform(a, "GET", "/api/catalog", cookie, "", "")
	require.Equal(t, 200, list.Code)
	assert.Contains(t, list.Body.String(), "documentation")
	assert.NotContains(t, list.Body.String(), `"$ref"`, "reader receives expandable field schemas")
	assert.NotContains(t, perform(a, "GET", "/api/site", "", "", "").Body.String(), "response_schema", "public site metadata stays lightweight")
}
