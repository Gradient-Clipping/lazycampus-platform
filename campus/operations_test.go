package campus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Gradient-Clipping/lazycampus-platform/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func responseData[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	require.Equal(t, 200, w.Code, w.Body.String())
	var envelope struct {
		Data T `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &envelope))
	return envelope.Data
}
func jsonBody(t *testing.T, v any) string {
	t.Helper()
	raw, err := common.Marshal(v)
	require.NoError(t, err)
	return string(raw)
}
func csrfFor(t *testing.T, a *App, cookie string) string {
	return responseData[struct {
		CSRF string `json:"csrf_token"`
	}](t, perform(a, "GET", "/api/me", cookie, "", "")).CSRF
}
func apiCall(a *App, key, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	req.RemoteAddr = "198.51.100.12:1234"
	req.Header.Set("Authorization", "Bearer "+key)
	rec := httptest.NewRecorder()
	a.Router().ServeHTTP(rec, req)
	return rec
}
func testKey(t *testing.T, a *App, u User) (Token, string) {
	t.Helper()
	key := "lc_" + randomKey(32)
	token := Token{UserID: u.ID, Name: "test app", KeyHash: digest(key), KeyPrefix: key[:12], Scopes: "timetable:read", Enabled: true, DailyQuota: 100, RateLimit: 60, Quota: 1000}
	require.NoError(t, a.DB.Create(&token).Error)
	return token, key
}
func TestOperationsDiagnosticsAndAnalyticIsolation(t *testing.T) {
	a, u, cookie := testApp(t)
	token, key := testKey(t, a, u)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "campus-test-id")
		w.WriteHeader(403)
		fmt.Fprint(w, `{"success":false,"error":{"code":"SCHOOL_IDENTITY_UNAVAILABLE","message":"private response details"}}`)
	}))
	defer upstream.Close()
	a.Config.CampusURL = upstream.URL
	assert.Equal(t, 403, apiCall(a, key, "/v1/teaching/grades").Code)
	assert.Equal(t, 400, apiCall(a, key, "/v1/teaching/timetable?private-school-password=SECRET").Code)
	assert.Equal(t, 403, apiCall(a, key, "/v1/teaching/timetable").Code)
	foreign := Log{UserID: u.ID + 1, TokenID: token.ID + 1, RequestID: randomKey(18), Status: 200, Admitted: true, Endpoint: "/teaching/timetable", DurationMS: 999}
	require.NoError(t, a.DB.Create(&foreign).Error)
	logs := responseData[struct {
		Items []Log
		Total int64
	}](t, perform(a, "GET", "/api/logs", cookie, "", ""))
	require.Len(t, logs.Items, 3)
	assert.True(t, logs.Items[0].Admitted)
	assert.Equal(t, "campus-test-id", logs.Items[0].UpstreamRequestID)
	assert.Equal(t, "SCHOOL_IDENTITY_UNAVAILABLE", logs.Items[0].ErrorCode)
	serialized := jsonBody(t, logs)
	assert.NotContains(t, serialized, "SECRET")
	assert.NotContains(t, serialized, "private response details")
	assert.NotContains(t, serialized, key)
	assert.Equal(t, 404, perform(a, "GET", fmt.Sprintf("/api/logs/%d", foreign.ID), cookie, "", "").Code)
	summary := responseData[struct {
		Summary Metrics
		Groups  map[string][]metricGroup
		Series  []metricPoint
	}](t, perform(a, "GET", "/api/analytics?user_id="+fmt.Sprint(foreign.UserID), cookie, "", ""))
	assert.EqualValues(t, 3, summary.Summary.Requests)
	assert.EqualValues(t, 1, summary.Summary.Admitted)
	assert.EqualValues(t, 2, summary.Summary.Rejected)
	require.Len(t, summary.Groups["applications"], 1)
	assert.EqualValues(t, 3, summary.Groups["applications"][0].Requests)
	require.NotEmpty(t, summary.Series)
	assert.EqualValues(t, 3, summary.Series[0].Requests)
	filtered := responseData[struct{ Total int64 }](t, perform(a, "GET", "/api/logs?status=rejected&error_code=SCOPE_REQUIRED", cookie, "", ""))
	assert.EqualValues(t, 1, filtered.Total)
	for _, q := range []string{"from=not-a-date", "status=evil", "token_id=abc", "min_ms=-1"} {
		assert.Equal(t, 400, perform(a, "GET", "/api/logs?"+q, cookie, "", "").Code)
	}
	require.NoError(t, a.DB.Model(&u).Update("role", "admin").Error)
	admin := responseData[struct{ Summary Metrics }](t, perform(a, "GET", "/api/admin/analytics", cookie, "", ""))
	assert.EqualValues(t, 4, admin.Summary.Requests)
}
func TestRuntimePolicyPreservesUsageAndCustomUsers(t *testing.T) {
	a, u, cookie := testApp(t)
	require.NoError(t, a.DB.Model(&u).Updates(map[string]any{"role": "admin", "inherit_limits": true}).Error)
	csrf := csrfFor(t, a, cookie)
	custom := User{Subject: randomKey(20), DailyQuota: 777, RateLimit: 7, Enabled: true}
	require.NoError(t, a.DB.Create(&custom).Error)
	usage := DailyUsage{Bucket: fmt.Sprintf("user:%d", u.ID), Day: time.Now().UTC().Format("2006-01-02"), Requests: 16}
	require.NoError(t, a.DB.Create(&usage).Error)
	current := responseData[settingsInput[PlatformPolicy]](t, perform(a, "GET", "/api/admin/settings/policy", cookie, "", ""))
	current.Value.DailyQuota = 30
	current.Value.RateLimit = 4
	current.Value.MaxApps = 1
	assert.Equal(t, 200, perform(a, "PUT", "/api/admin/settings/policy", cookie, csrf, jsonBody(t, current)).Code)
	assert.Equal(t, 409, perform(a, "PUT", "/api/admin/settings/policy", cookie, csrf, jsonBody(t, current)).Code)
	require.NoError(t, a.DB.First(&u, u.ID).Error)
	assert.EqualValues(t, 30, u.DailyQuota)
	assert.EqualValues(t, 4, u.RateLimit)
	require.NoError(t, a.DB.First(&custom, custom.ID).Error)
	assert.EqualValues(t, 777, custom.DailyQuota)
	require.NoError(t, a.DB.First(&usage, "bucket = ? AND day = ?", usage.Bucket, usage.Day).Error)
	assert.EqualValues(t, 16, usage.Requests)
	body := `{"name":"runtime defaults","scopes":["timetable:read"],"daily_quota":30,"rate_limit":4,"quota":1000,"expires_in_days":30}`
	assert.Equal(t, 200, perform(a, "POST", "/api/tokens", cookie, csrf, body).Code)
	assert.Equal(t, 400, perform(a, "POST", "/api/tokens", cookie, csrf, body).Code)
	public := responseData[struct{ Limits PlatformPolicy }](t, perform(a, "GET", "/api/site", "", "", ""))
	assert.EqualValues(t, 30, public.Limits.DailyQuota)
	require.NoError(t, Migrate(a.DB))
	p, err := a.policy()
	require.NoError(t, err)
	assert.EqualValues(t, 30, p.DailyQuota)
}
func TestAdminAppRegulationAndEndpointPolicies(t *testing.T) {
	a, u, cookie := testApp(t)
	token, key := testKey(t, a, u)
	csrf := csrfFor(t, a, cookie)
	path := fmt.Sprintf("/api/admin/tokens/%d", token.ID)
	assert.Equal(t, 403, perform(a, "PATCH", path, cookie, csrf, `{"action":"suspend","reason":"test"}`).Code)
	require.NoError(t, a.DB.Model(&u).Update("role", "admin").Error)
	assert.Equal(t, 200, perform(a, "PATCH", path, cookie, csrf, `{"action":"suspend","reason":"异常调用"}`).Code)
	assert.Equal(t, 401, apiCall(a, key, "/v1/teaching/timetable").Code)
	edit := `{"name":"owner edit","scopes":["timetable:read"],"daily_quota":100,"rate_limit":20,"quota":1000,"never_expires":true,"enabled":true}`
	assert.Equal(t, 200, perform(a, "PATCH", fmt.Sprintf("/api/tokens/%d", token.ID), cookie, csrf, edit).Code)
	assert.Equal(t, 401, apiCall(a, key, "/v1/teaching/timetable").Code)
	assert.Equal(t, 200, perform(a, "PATCH", path, cookie, csrf, `{"action":"resume","reason":"已处理"}`).Code)
	var endpoint EndpointPolicy
	require.NoError(t, a.DB.First(&endpoint, "path = ?", "/teaching/timetable").Error)
	endpoint.Status = "maintenance"
	assert.Equal(t, 200, perform(a, "PUT", "/api/admin/endpoints", cookie, csrf, jsonBody(t, endpoint)).Code)
	assert.Equal(t, 503, apiCall(a, key, "/v1/teaching/timetable").Code)
	require.NoError(t, a.DB.First(&endpoint, "path = ?", endpoint.Path).Error)
	endpoint.Status = "operational"
	endpoint.Scope = "grades:read"
	assert.Equal(t, 200, perform(a, "PUT", "/api/admin/endpoints", cookie, csrf, jsonBody(t, endpoint)).Code)
	assert.Equal(t, 403, apiCall(a, key, "/v1/teaching/timetable").Code)
	require.NoError(t, a.DB.First(&endpoint, "path = ?", endpoint.Path).Error)
	endpoint.Scope = "timetable:read"
	endpoint.DailyQuota = 1
	assert.Equal(t, 200, perform(a, "PUT", "/api/admin/endpoints", cookie, csrf, jsonBody(t, endpoint)).Code)
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true}`)
	}))
	defer service.Close()
	a.Config.CampusURL = service.URL
	assert.Equal(t, 200, apiCall(a, key, "/v1/teaching/timetable").Code)
	assert.Equal(t, 429, apiCall(a, key, "/v1/teaching/timetable").Code)
	assert.Equal(t, 200, perform(a, "PATCH", path, cookie, csrf, `{"action":"revoke","reason":"撤销测试"}`).Code)
	assert.Equal(t, 400, perform(a, "PATCH", path, cookie, csrf, `{"action":"resume","reason":"不应恢复"}`).Code)
	assert.Equal(t, 400, perform(a, "PATCH", fmt.Sprintf("/api/tokens/%d", token.ID), cookie, csrf, edit).Code)
	assert.Equal(t, 404, perform(a, "POST", fmt.Sprintf("/api/tokens/%d/rotate", token.ID), cookie, csrf, "").Code)
	require.NoError(t, a.DB.First(&token, token.ID).Error)
	assert.EqualValues(t, 1, token.UsedQuota)
}

type senderTransport func(*http.Request) (*http.Response, error)

func TestWebPBrandMigrationPreservesSiteContent(t *testing.T) {
	a, _, _ := testApp(t)
	require.NoError(t, a.DB.Where("name = ?", "migration.logo-webp.v1").Delete(&Setting{}).Error)
	legacy := map[string]any{"logo_url": "/logo.png", "name": "学校开放平台", "future_field": "preserved"}
	require.NoError(t, a.DB.Model(&Setting{}).Where("name = ?", "operations.site.v1").Update("value", jsonBody(t, legacy)).Error)
	require.NoError(t, migrateOperations(a.DB))
	updated, err := readSetting(a.DB, "operations.site.v1", map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "/logo.webp", updated["logo_url"])
	assert.Equal(t, legacy["name"], updated["name"])
	assert.Equal(t, "preserved", updated["future_field"])
	updated["logo_url"] = "/custom-brand.webp"
	require.NoError(t, a.DB.Model(&Setting{}).Where("name = ?", "operations.site.v1").Update("value", jsonBody(t, updated)).Error)
	require.NoError(t, migrateOperations(a.DB))
	updated, err = readSetting(a.DB, "operations.site.v1", map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "/custom-brand.webp", updated["logo_url"])
}

func (f senderTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestPersonalAlertsEmailVerificationAndDeduplication(t *testing.T) {
	a, u, cookie := testApp(t)
	csrf := csrfFor(t, a, cookie)
	a.Config.SenderAPIKey = "test-key"
	a.Config.SenderFrom = "alerts@example.com"
	var sentCode string
	sent := 0
	a.HTTP = &http.Client{Transport: senderTransport(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "https://api.sender.net/v2/message/send", r.URL.String())
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		var payload struct {
			To      struct{ Email string }
			Subject string
			Text    string
		}
		raw, _ := io.ReadAll(r.Body)
		require.NoError(t, common.Unmarshal(raw, &payload))
		assert.Equal(t, "student@example.com", payload.To.Email)
		if strings.Contains(payload.Subject, "验证") {
			sentCode = strings.TrimPrefix(strings.Split(payload.Text, "\n")[0], "您的验证码是：")
		}
		sent++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"success":true,"emailId":"fake-id"}`)), Header: make(http.Header)}, nil
	})}
	p, err := a.notificationPreference(u.ID)
	require.NoError(t, err)
	p.Email = "student@example.com"
	p.EmailEnabled = true
	assert.Equal(t, 400, perform(a, "PUT", "/api/notification-preferences", cookie, csrf, jsonBody(t, p)).Code)
	p.EmailEnabled = false
	assert.Equal(t, 200, perform(a, "PUT", "/api/notification-preferences", cookie, csrf, jsonBody(t, p)).Code)
	assert.Equal(t, 200, perform(a, "POST", "/api/notification-preferences/send-verification", cookie, csrf, "").Code)
	require.Len(t, sentCode, 6)
	assert.Equal(t, 400, perform(a, "POST", "/api/notification-preferences/verify", cookie, csrf, `{"code":"abcdef"}`).Code)
	assert.Equal(t, 200, perform(a, "POST", "/api/notification-preferences/verify", cookie, csrf, `{"code":"`+sentCode+`"}`).Code)
	p.EmailEnabled = true
	assert.Equal(t, 200, perform(a, "PUT", "/api/notification-preferences", cookie, csrf, jsonBody(t, p)).Code)
	now := time.Now().UTC()
	require.NoError(t, a.DB.Create(&DailyUsage{Bucket: fmt.Sprintf("user:%d", u.ID), Day: now.Format("2006-01-02"), Requests: 4500}).Error)
	require.NoError(t, a.checkUserAlerts(u, now))
	require.NoError(t, a.checkUserAlerts(u, now))
	var count int64
	require.NoError(t, a.DB.Model(&Notification{}).Where("user_id = ? AND kind = ?", u.ID, "quota").Count(&count).Error)
	assert.EqualValues(t, 1, count)
	a.deliverNotifications(context.Background())
	a.deliverNotifications(context.Background())
	assert.Equal(t, 2, sent)
	messages := responseData[struct {
		Items []Notification
		Total int64
	}](t, perform(a, "GET", "/api/notifications", cookie, "", ""))
	require.Len(t, messages.Items, 1)
	assert.Equal(t, "sent", messages.Items[0].EmailStatus)
	assert.Equal(t, 200, perform(a, "POST", "/api/notifications/all/read", cookie, csrf, "").Code)
	unread := responseData[struct{ Count int }](t, perform(a, "GET", "/api/notifications/unread", cookie, "", ""))
	assert.Zero(t, unread.Count)
	p.Email = "changed@example.com"
	p.EmailEnabled = false
	assert.Equal(t, 200, perform(a, "PUT", "/api/notification-preferences", cookie, csrf, jsonBody(t, p)).Code)
	p.EmailEnabled = true
	assert.Equal(t, 400, perform(a, "PUT", "/api/notification-preferences", cookie, csrf, jsonBody(t, p)).Code)
	a.HTTP = &http.Client{Transport: senderTransport(func(*http.Request) (*http.Response, error) { return nil, errors.New("sensitive transport diagnostic") })}
	assert.ErrorIs(t, a.sendMail(context.Background(), "student@example.com", "test", "test"), errEmailUncertain)
}
func TestDeviceRevocationIsScopedAndKeepsCurrentSession(t *testing.T) {
	a, u, cookie := testApp(t)
	csrf := csrfFor(t, a, cookie)
	otherCookie := randomKey(32)
	other := browserSession{UserID: u.ID, Subject: u.Subject, SID: "other-provider-session", CSRF: randomKey(32), RefreshToken: "secret-refresh-token", ExpiresAt: time.Now().Add(time.Hour).Unix(), RefreshAt: time.Now().Add(time.Hour).Unix()}
	require.NoError(t, a.Redis.Set(context.Background(), "platform:v1:session:"+digest(otherCookie), jsonBody(t, other), time.Hour).Err())
	assert.Equal(t, 200, perform(a, "GET", "/api/me", otherCookie, "", "").Code)
	providerCalls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls++
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "secret-refresh-token", r.Form.Get("refresh_token"))
		w.WriteHeader(204)
	}))
	defer provider.Close()
	a.Config.Issuer = provider.URL
	devices := responseData[struct {
		Items     []SessionDevice
		CurrentID string `json:"current_id"`
	}](t, perform(a, "GET", "/api/sessions", cookie, "", ""))
	require.Len(t, devices.Items, 2)
	assert.Equal(t, digest(cookie), devices.CurrentID)
	assert.NotContains(t, jsonBody(t, devices), otherCookie)
	assert.NotContains(t, jsonBody(t, devices), other.RefreshToken)
	foreign := SessionDevice{ID: digest(randomKey(32)), UserID: u.ID + 1, LastActiveAt: time.Now().UTC(), ExpiresAt: time.Now().Add(time.Hour)}
	require.NoError(t, a.DB.Create(&foreign).Error)
	assert.Equal(t, 404, perform(a, "DELETE", "/api/sessions/"+foreign.ID, cookie, csrf, "").Code)
	assert.Equal(t, 200, perform(a, "POST", "/api/sessions/revoke-others", cookie, csrf, "").Code)
	assert.Equal(t, 1, providerCalls)
	assert.Equal(t, 401, perform(a, "GET", "/api/me", otherCookie, "", "").Code)
	assert.Equal(t, 200, perform(a, "GET", "/api/me", cookie, "", "").Code)
	// A concurrent refresh may restore Redis bytes but cannot restore a revoked DB grant.
	require.NoError(t, a.Redis.Set(context.Background(), "platform:v1:session:"+digest(otherCookie), jsonBody(t, other), time.Hour).Err())
	assert.Equal(t, 401, perform(a, "GET", "/api/me", otherCookie, "", "").Code)
}
