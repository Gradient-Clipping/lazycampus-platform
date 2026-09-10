package campus

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Gradient-Clipping/lazycampus-platform/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlatformVisitRequiresSessionAndCSRFAndDoesNotTrackBackgroundReads(t *testing.T) {
	a, u, cookie := testApp(t)
	old := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Second)
	require.NoError(t, a.DB.Model(&u).Update("last_login_at", old).Error)
	var session struct {
		Data struct {
			CSRF string `json:"csrf_token"`
		}
	}
	require.NoError(t, common.Unmarshal(perform(a, "GET", "/api/me", cookie, "", "").Body.Bytes(), &session))
	assert.Equal(t, 401, perform(a, "POST", "/api/visit", "", "", "").Code)
	assert.Equal(t, 403, perform(a, "POST", "/api/visit", cookie, "", "").Code)
	require.NoError(t, a.DB.First(&u, u.ID).Error)
	assert.True(t, old.Equal(u.LastLoginAt), "session reads must not report a new platform login")
	before := time.Now().UTC().Add(-time.Second)
	response := perform(a, "POST", "/api/visit", cookie, session.Data.CSRF, "")
	require.Equal(t, 200, response.Code)
	require.NoError(t, a.DB.First(&u, u.ID).Error)
	assert.True(t, u.LastLoginAt.After(before))
	recorded := u.LastLoginAt
	assert.Equal(t, 200, perform(a, "GET", "/api/me", cookie, "", "").Code)
	assert.Equal(t, 200, perform(a, "GET", "/api/dashboard", cookie, "", "").Code)
	require.NoError(t, a.DB.First(&u, u.ID).Error)
	assert.True(t, recorded.Equal(u.LastLoginAt))
}

func TestLongTermApplicationLifecyclePreservesQuotaAndExpiration(t *testing.T) {
	a, u, cookie := testApp(t)
	var session struct {
		Data struct {
			CSRF string `json:"csrf_token"`
		}
	}
	require.NoError(t, common.Unmarshal(perform(a, "GET", "/api/me", cookie, "", "").Body.Bytes(), &session))
	body := `{"name":"long term","scopes":["timetable:read"],"daily_quota":2,"rate_limit":20,"quota":10,"expires_in_days":0,"never_expires":true}`
	response := perform(a, "POST", "/api/tokens", cookie, session.Data.CSRF, body)
	require.Equal(t, 200, response.Code, response.Body.String())
	var created struct {
		Data struct {
			Token Token
			Key   string
		}
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &created))
	token := created.Data.Token
	assert.Nil(t, token.ExpiresAt)
	require.NoError(t, Migrate(a.DB))
	var stored Token
	require.NoError(t, a.DB.First(&stored, token.ID).Error)
	assert.Nil(t, stored.ExpiresAt)
	var dashboard struct {
		Data struct {
			ActiveApps int `json:"active_apps"`
		}
	}
	require.NoError(t, common.Unmarshal(perform(a, "GET", "/api/dashboard", cookie, "", "").Body.Bytes(), &dashboard))
	assert.Equal(t, 1, dashboard.Data.ActiveApps)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"data":{}}`)
	}))
	defer upstream.Close()
	a.Config.CampusURL = upstream.URL
	call := func(key string) int {
		request := httptest.NewRequest("GET", "/v1/teaching/timetable", nil)
		request.Header.Set("Authorization", "Bearer "+key)
		request.RemoteAddr = "198.51.100.12:1234"
		result := httptest.NewRecorder()
		a.Router().ServeHTTP(result, request)
		return result.Code
	}
	assert.Equal(t, 200, call(created.Data.Key), "a NULL expiry must pass the gateway")
	require.NoError(t, a.DB.First(&u, u.ID).Error)
	assert.True(t, u.LastLoginAt.Equal(u.CreatedAt), "API calls must not update platform login time")
	rotated := perform(a, "POST", fmt.Sprintf("/api/tokens/%d/rotate", token.ID), cookie, session.Data.CSRF, "")
	require.Equal(t, 200, rotated.Code)
	var rotation struct{ Data struct{ Key string } }
	require.NoError(t, common.Unmarshal(rotated.Body.Bytes(), &rotation))
	assert.Equal(t, 401, call(created.Data.Key))
	assert.Equal(t, 200, call(rotation.Data.Key))
	assert.Equal(t, 429, call(rotation.Data.Key), "long-term keys remain subject to daily quota")
	require.NoError(t, a.DB.First(&stored, token.ID).Error)
	assert.Nil(t, stored.ExpiresAt)
	assert.EqualValues(t, 2, stored.UsedQuota)
	finite := `{"name":"fixed term","scopes":["timetable:read"],"daily_quota":2,"rate_limit":20,"quota":10,"expires_in_days":30,"never_expires":false}`
	endpoint := fmt.Sprintf("/api/tokens/%d", token.ID)
	require.Equal(t, 200, perform(a, "PATCH", endpoint, cookie, session.Data.CSRF, finite).Code)
	stored = Token{}
	require.NoError(t, a.DB.First(&stored, token.ID).Error)
	require.NotNil(t, stored.ExpiresAt)
	assert.WithinDuration(t, time.Now().Add(30*24*time.Hour), *stored.ExpiresAt, time.Minute)
	expiry := *stored.ExpiresAt
	require.NoError(t, Migrate(a.DB))
	require.NoError(t, a.DB.First(&stored, token.ID).Error)
	assert.True(t, expiry.Equal(*stored.ExpiresAt), "restart must retain existing finite expirations")
	require.NoError(t, a.DB.Model(&stored).Update("expires_at", time.Now().Add(-time.Hour)).Error)
	assert.Equal(t, 401, call(rotation.Data.Key))
	_, err := a.reserve(context.Background(), u.ID, token.ID, &Log{UserID: u.ID})
	assert.ErrorIs(t, err, errDisabled)
	require.Equal(t, 200, perform(a, "PATCH", endpoint, cookie, session.Data.CSRF, body).Code)
	stored = Token{}
	require.NoError(t, a.DB.First(&stored, token.ID).Error)
	assert.Nil(t, stored.ExpiresAt)
	assert.EqualValues(t, 2, stored.UsedQuota)
	assert.Equal(t, 429, call(rotation.Data.Key))
}
