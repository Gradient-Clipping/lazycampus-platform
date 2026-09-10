package campus

import (
	"context"
	"fmt"
	"github.com/Gradient-Clipping/lazycampus-platform/common"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDailyQuota1000MigrationPreservesCustomLimitsAndUsage(t *testing.T) {
	db := testDatabase(t)
	require.NoError(t, Migrate(db))
	require.NoError(t, db.Where("name = ?", "migration.daily-quota.v3").Delete(&Setting{}).Error)
	policy := defaultPolicy()
	policy.DailyQuota = 2000
	raw, err := common.Marshal(policy)
	require.NoError(t, err)
	require.NoError(t, db.Model(&Setting{}).Where("name = ?", "operations.policy.v1").Update("value", string(raw)).Error)
	users := []User{
		{Subject: randomKey(20), Enabled: true, DailyQuota: 2000, RateLimit: 20, InheritLimits: true},
		{Subject: randomKey(20), Enabled: true, DailyQuota: 3000, RateLimit: 35},
	}
	require.NoError(t, db.Create(&users).Error)
	tokens := []Token{
		{UserID: users[0].ID, KeyHash: digest(randomKey(32)), DailyQuota: 2000, UsedQuota: 1700},
		{UserID: users[0].ID, KeyHash: digest(randomKey(32)), DailyQuota: 500},
		{UserID: users[1].ID, KeyHash: digest(randomKey(32)), DailyQuota: 2000},
	}
	require.NoError(t, db.Create(&tokens).Error)
	usage := DailyUsage{Bucket: fmt.Sprintf("user:%d", users[0].ID), Day: time.Now().UTC().Format("2006-01-02"), Requests: 1700}
	require.NoError(t, db.Create(&usage).Error)
	require.NoError(t, db.Model(&EndpointPolicy{}).Where("path = ?", Catalog[0].Path).Update("daily_quota", 2000).Error)
	require.NoError(t, Migrate(db))
	for i, expected := range []int64{1000, 3000} {
		var user User
		require.NoError(t, db.First(&user, users[i].ID).Error)
		assert.Equal(t, expected, user.DailyQuota)
	}
	for i, expected := range []int64{1000, 500, 2000} {
		var token Token
		require.NoError(t, db.First(&token, tokens[i].ID).Error)
		assert.Equal(t, expected, token.DailyQuota)
		assert.Equal(t, tokens[i].UsedQuota, token.UsedQuota)
	}
	var after DailyUsage
	require.NoError(t, db.Where("bucket = ? AND day = ?", usage.Bucket, usage.Day).First(&after).Error)
	assert.Equal(t, usage.Requests, after.Requests)
	app := &App{DB: db}
	saved, err := app.policy()
	require.NoError(t, err)
	assert.EqualValues(t, 1000, saved.DailyQuota)
	require.NoError(t, db.Model(&users[0]).Update("daily_quota", 1400).Error)
	require.NoError(t, Migrate(db))
	require.NoError(t, db.First(&users[0], users[0].ID).Error)
	assert.EqualValues(t, 1400, users[0].DailyQuota)
}

func TestDailyQuotaUpgradePreservesUsageAndCustomLimits(t *testing.T) {
	db := testDatabase(t)
	var err error
	require.NoError(t, Migrate(db))
	// Simulate a populated installation that already completed the rate migration.
	require.NoError(t, db.Where("name = ?", "migration.daily-quota.v2").Delete(&Setting{}).Error)
	users := []User{
		{Subject: randomKey(20), Enabled: true, DailyQuota: 5000, RateLimit: 20},
		{Subject: randomKey(20), Enabled: true, DailyQuota: 4200, RateLimit: 20},
	}
	require.NoError(t, db.Create(&users).Error)
	tokens := []Token{
		{UserID: users[0].ID, KeyHash: digest(randomKey(32)), Enabled: true, DailyQuota: 3000, Quota: 100000, UsedQuota: 2500, ExpiresAt: timePointer(time.Now().Add(time.Hour))},
		{UserID: users[0].ID, KeyHash: digest(randomKey(32)), DailyQuota: 1000, ExpiresAt: timePointer(time.Now().Add(time.Hour))},
		{UserID: users[1].ID, KeyHash: digest(randomKey(32)), DailyQuota: 3000, ExpiresAt: timePointer(time.Now().Add(time.Hour))},
	}
	require.NoError(t, db.Create(&tokens).Error)
	usage := DailyUsage{Bucket: fmt.Sprintf("user:%d", users[0].ID), Day: time.Now().UTC().Format("2006-01-02"), Requests: 2500}
	require.NoError(t, db.Create(&usage).Error)
	require.NoError(t, Migrate(db))
	var upgraded User
	require.NoError(t, db.First(&upgraded, users[0].ID).Error)
	assert.EqualValues(t, 1000, upgraded.DailyQuota)
	var custom User
	require.NoError(t, db.First(&custom, users[1].ID).Error)
	assert.EqualValues(t, 4200, custom.DailyQuota)
	for i, expected := range []int64{1000, 1000, 3000} {
		var token Token
		require.NoError(t, db.First(&token, tokens[i].ID).Error)
		assert.Equal(t, expected, token.DailyQuota)
		assert.Equal(t, tokens[i].UsedQuota, token.UsedQuota)
	}
	app := &App{DB: db}
	_, err = app.reserve(context.Background(), users[0].ID, tokens[0].ID, &Log{UserID: users[0].ID})
	assert.ErrorIs(t, err, errQuota, "usage above the new quota must remain exhausted until reset")
	var after DailyUsage
	require.NoError(t, db.Where("bucket = ? AND day = ?", usage.Bucket, usage.Day).First(&after).Error)
	assert.EqualValues(t, 2500, after.Requests)
	// Restarting must not overwrite a later explicit administrator adjustment.
	require.NoError(t, db.Model(&upgraded).Update("daily_quota", 5000).Error)
	require.NoError(t, Migrate(db))
	require.NoError(t, db.First(&upgraded, users[0].ID).Error)
	assert.EqualValues(t, 5000, upgraded.DailyQuota)
}
