package campus

import (
	"fmt"
	"github.com/Gradient-Clipping/lazycampus-platform/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestCampusServiceLimitsUpgradeIsOnceAndPreservesUsage(t *testing.T) {
	db := testDatabase(t)
	require.NoError(t, Migrate(db))
	require.NoError(t, db.Where("name = ?", "migration.campus-services-limits.v1").Delete(&Setting{}).Error)
	policy := defaultPolicy()
	policy.DailyQuota = 1000
	policy.RateLimit = 20
	raw, err := common.Marshal(policy)
	require.NoError(t, err)
	require.NoError(t, db.Model(&Setting{}).Where("name = ?", "operations.policy.v1").Update("value", string(raw)).Error)
	users := []User{{Subject: randomKey(20), DailyQuota: 1000, RateLimit: 20, InheritLimits: true}, {Subject: randomKey(20), DailyQuota: 800, RateLimit: 12}}
	require.NoError(t, db.Create(&users).Error)
	tokens := []Token{{UserID: users[0].ID, KeyHash: digest(randomKey(32)), DailyQuota: 1000, RateLimit: 20, UsedQuota: 987}, {UserID: users[0].ID, KeyHash: digest(randomKey(32)), DailyQuota: 500, RateLimit: 10, UsedQuota: 99}, {UserID: users[1].ID, KeyHash: digest(randomKey(32)), DailyQuota: 1000, RateLimit: 20}}
	require.NoError(t, db.Create(&tokens).Error)
	usage := DailyUsage{Bucket: fmt.Sprintf("user:%d", users[0].ID), Day: time.Now().UTC().Format("2006-01-02"), Requests: 987}
	require.NoError(t, db.Create(&usage).Error)
	require.NoError(t, db.Model(&EndpointPolicy{}).Where("path IN ?", []string{"/teaching/pass-rates", "/teaching/grades/class-distribution"}).Updates(map[string]any{"daily_quota": 1000, "rate_limit": 20, "cooldown": 10}).Error)
	require.NoError(t, Migrate(db))
	for i, want := range []struct{ daily, rate int64 }{{1500, 30}, {800, 12}} {
		var got User
		require.NoError(t, db.First(&got, users[i].ID).Error)
		assert.Equal(t, want.daily, got.DailyQuota)
		assert.Equal(t, want.rate, got.RateLimit)
	}
	for i, want := range []struct{ daily, rate int64 }{{1500, 30}, {500, 10}, {1000, 20}} {
		var got Token
		require.NoError(t, db.First(&got, tokens[i].ID).Error)
		assert.Equal(t, want.daily, got.DailyQuota)
		assert.Equal(t, want.rate, got.RateLimit)
		assert.Equal(t, tokens[i].UsedQuota, got.UsedQuota)
	}
	var after DailyUsage
	require.NoError(t, db.Where("bucket = ? AND day = ?", usage.Bucket, usage.Day).First(&after).Error)
	assert.Equal(t, usage.Requests, after.Requests)
	var endpoints []EndpointPolicy
	require.NoError(t, db.Where("path IN ?", []string{"/teaching/pass-rates", "/teaching/grades/class-distribution"}).Find(&endpoints).Error)
	for _, e := range endpoints {
		assert.EqualValues(t, 1500, e.DailyQuota)
		assert.EqualValues(t, 30, e.RateLimit)
		assert.Zero(t, e.Cooldown)
	}
	require.NoError(t, db.Model(&users[0]).Updates(map[string]any{"daily_quota": 1700, "rate_limit": 35}).Error)
	require.NoError(t, db.Model(&EndpointPolicy{}).Where("path = ?", "/teaching/pass-rates").Update("cooldown", 3).Error)
	require.NoError(t, Migrate(db))
	require.NoError(t, db.First(&users[0], users[0].ID).Error)
	assert.EqualValues(t, 1700, users[0].DailyQuota)
	assert.EqualValues(t, 35, users[0].RateLimit)
	var endpoint EndpointPolicy
	require.NoError(t, db.First(&endpoint, "path = ?", "/teaching/pass-rates").Error)
	assert.Equal(t, 3, endpoint.Cooldown)
}
