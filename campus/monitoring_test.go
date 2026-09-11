package campus

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMonitoringReadiness(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		code       int
		want       bool
	}{
		{"ready", `{"success":true,"data":{"status":"ready"}}`, 200, true},
		{"unready", `{"success":false,"data":{"status":"not_ready"}}`, 503, false},
		{"wrong-page", `<html>login</html>`, 200, false},
		{"redirect", ``, 302, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/api/v1/system/ready", r.URL.Path)
				assert.Empty(t, r.Header.Get("Authorization"))
				assert.Empty(t, r.Header.Get("X-Campus-Signature"))
				if tc.code == 302 {
					w.Header().Set("Location", "/must-not-follow")
				}
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			assert.Equal(t, tc.want, monitoringCampusReady(context.Background(), server.Client(), server.URL))
		})
	}
	assert.Equal(t, "operational", monitoringWithHealth(monitoringTraffic(nil), "operational")["status"])
	assert.Equal(t, "no_data", monitoringWithHealth(gin.H{"status": "no_data"}, "operational")["status"])
	assert.Equal(t, "full_outage", monitoringWithHealth(monitoringTraffic(nil), "full_outage")["status"])
	assert.NotContains(t, monitoringTraffic(make([]Log, 20000)), "requests")
}

func TestMonitoringTraffic(t *testing.T) {
	assert.Equal(t, "no_data", monitoringTraffic([]Log{{Status: 401}, {Status: 429}})["status"])
	assert.Equal(t, 1, monitoringTraffic([]Log{{Status: 429}})["limited"])
	assert.Equal(t, "partial_outage", monitoringTraffic([]Log{{Status: 502}, {Status: 503}, {Status: 500}})["status"])
	assert.Equal(t, "operational", monitoringTraffic([]Log{{Status: 200, DurationMS: 50}})["status"])
	assert.Equal(t, "no_data", monitoringTraffic([]Log{{Status: 503, ErrorCode: "ENDPOINT_UNAVAILABLE"}})["status"])
}
