package campus

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnlimitedTotalQuotaStillEnforcesDailyQuotaAndTracksUsage(t *testing.T) {
	a, user, cookie := testApp(t)
	csrf := csrfFor(t, a, cookie)
	input := tokenInput{Name: "Unlimited app", Scopes: []string{"timetable:read"}, DailyQuota: 2, RateLimit: 20, Quota: 0, NeverExpires: true}
	created := responseData[struct {
		Token Token  `json:"token"`
		Key   string `json:"key"`
	}](t, perform(a, "POST", "/api/tokens", cookie, csrf, jsonBody(t, input)))
	var token Token
	require.NoError(t, a.DB.Where("key_hash = ?", digest(created.Key)).First(&token).Error)
	assert.Zero(t, token.Quota)
	require.NoError(t, a.DB.Model(&token).Update("used_quota", 10000001).Error)
	for i := range 2 {
		remaining, err := a.reserve(context.Background(), user.ID, token.ID, &Log{UserID: user.ID, TokenID: token.ID, Endpoint: "/teaching/timetable", RequestID: randomKey(16)})
		require.NoError(t, err)
		assert.EqualValues(t, 1-i, remaining)
	}
	entry := &Log{UserID: user.ID, TokenID: token.ID, Endpoint: "/teaching/timetable"}
	_, err := a.reserve(context.Background(), user.ID, token.ID, entry)
	assert.ErrorIs(t, err, errQuota)
	assert.Equal(t, "token_daily", entry.LimitScope)
	require.NoError(t, a.DB.First(&token, token.ID).Error)
	assert.EqualValues(t, 10000003, token.UsedQuota)
	// A finite quota cannot be reduced below recorded usage, even after unlimited use.
	input.Quota = 100
	route := fmt.Sprintf("/api/tokens/%d", token.ID)
	assert.Equal(t, 400, perform(a, "PATCH", route, cookie, csrf, jsonBody(t, input)).Code)
	input.Quota = 0
	enabled := true
	input.Enabled = &enabled
	assert.Equal(t, 200, perform(a, "PATCH", route, cookie, csrf, jsonBody(t, input)).Code)
	pref := defaultNotificationPreference(user.ID)
	require.NoError(t, a.DB.Create(&pref).Error)
	require.NoError(t, a.checkUserAlerts(user, time.Now().UTC()))
	var notices int64
	require.NoError(t, a.DB.Model(&Notification{}).Where("user_id = ? AND title = ?", user.ID, "应用总额度提醒").Count(&notices).Error)
	assert.Zero(t, notices, "unlimited total quotas must not emit exhausted-total alerts")
}
