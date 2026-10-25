package campus

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayCertificateMethodBodySignatureAndScope(t *testing.T) {
	a, u, _ := testApp(t)
	token, key := testKey(t, a, u)
	require.NoError(t, a.DB.Model(&token).Update("scopes", "documents:read").Error)
	calls := 0
	body := `{"selected":{"language":"中文<test>"}}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Equal(t, body, string(raw))
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/internal/platform/v1/campus-documents/certificates/transcript/issue", r.URL.RequestURI())
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Empty(t, r.Header.Get("Authorization"))
		canonical := strings.Join([]string{r.Method, r.URL.RequestURI(), r.Header.Get("X-Platform-Timestamp"), r.Header.Get("X-Platform-Nonce"), u.IdentityID, digest(string(raw))}, "\n")
		mac := hmac.New(sha256.New, []byte(a.Config.CampusSecret))
		mac.Write([]byte(canonical))
		assert.Equal(t, hex.EncodeToString(mac.Sum(nil)), r.Header.Get("X-Platform-Signature"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		fmt.Fprint(w, `{"success":true,"data":{"jobId":"job-1"}}`)
	}))
	defer upstream.Close()
	a.Config.CampusURL = upstream.URL
	call := func(method, path, payload string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(payload))
		r.Header.Set("Authorization", "Bearer "+key)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		a.Router().ServeHTTP(w, r)
		return w
	}
	path := "/v1/campus-documents/certificates/transcript/issue"
	assert.Equal(t, 403, call("POST", path, body).Code)
	require.NoError(t, a.DB.Model(&token).Update("scopes", "documents:read documents:issue").Error)
	for _, invalid := range []string{`null`, `[]`, `{"userId":999}`, `{"selected":null}`, `{"selected":{"x":null}}`, `{"selected":{"x":3}}`, `{} {}`} {
		assert.Equal(t, 400, call("POST", path, invalid).Code, invalid)
	}
	assert.Equal(t, 404, call("GET", path, "").Code)
	assert.Equal(t, 400, call("POST", path+"?id=other", body).Code)
	response := call("POST", path, body)
	require.Equal(t, 202, response.Code, response.Body.String())
	assert.Equal(t, 1, calls)
	var log Log
	require.NoError(t, a.DB.Where("user_id = ? AND admitted = ?", u.ID, true).First(&log).Error)
	assert.Equal(t, "/campus-documents/certificates/{id}/issue", log.Endpoint)
	assert.Equal(t, "POST", log.Method)
	assert.NotContains(t, log.Endpoint, "transcript")
}

func TestGatewayDynamicIDsAndPDFDownload(t *testing.T) {
	a, u, _ := testApp(t)
	a.HTTP.Timeout = 10 * time.Second
	token, key := testKey(t, a, u)
	require.NoError(t, a.DB.Model(&token).Update("scopes", "documents:read companions:read").Error)
	payload := "%PDF-" + strings.Repeat("a", 17*1024*1024)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/files/") {
			w.Header().Set("Content-Type", "application/pdf")
			fmt.Fprint(w, payload)
			return
		}
		assert.Equal(t, "/internal/platform/v1/teaching/companions/123/timetable?semester=2026-1", r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":null}`)
	}))
	defer upstream.Close()
	a.Config.CampusURL = upstream.URL
	call := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Authorization", "Bearer "+key)
		w := httptest.NewRecorder()
		a.Router().ServeHTTP(w, r)
		return w
	}
	response := call("/v1/campus-documents/files/owned-file")
	require.Equal(t, 200, response.Code, response.Body.String()[:min(100, response.Body.Len())])
	assert.Equal(t, "application/pdf", response.Header().Get("Content-Type"))
	assert.Equal(t, len(payload), response.Body.Len())
	assert.Equal(t, 200, call("/v1/teaching/companions/123/timetable?semester=2026-1").Code)
	assert.Equal(t, 400, call("/v1/teaching/companions/123/timetable?id=124").Code)
	assert.Equal(t, 404, call("/v1/teaching/companions/123/../124/timetable").Code)
}
