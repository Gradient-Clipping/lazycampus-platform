package campus

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var errQuota = errors.New("quota exhausted")
var errDisabled = errors.New("credential disabled")
var errEndpoint = errors.New("endpoint changed or unavailable")
var rateScript = redis.NewScript(`
for i,key in ipairs(KEYS) do
  if tonumber(redis.call('GET',key) or '0') >= tonumber(ARGV[i*2-1]) then
    return {0, math.max(1,redis.call('TTL',key)), i}
  end
end
for i,key in ipairs(KEYS) do
  if redis.call('INCR',key) == 1 then redis.call('EXPIRE',key,ARGV[i*2]) end
end
return {1,0}
`)
var concurrentScript = redis.NewScript(`
for i,key in ipairs(KEYS) do
 redis.call('ZREMRANGEBYSCORE',key,'-inf',ARGV[1])
 if redis.call('ZCARD',key)>=tonumber(ARGV[i+3]) then return 0 end
end
for i,key in ipairs(KEYS) do
 redis.call('ZADD',key,ARGV[2],ARGV[3]);redis.call('EXPIRE',key,100)
end
return 1
`)

func (a *App) managementRate(c *gin.Context, key string, limit int) bool {
	result, err := rateScript.Run(c.Request.Context(), a.Redis, []string{"platform:v1:rate:" + key}, limit, 60).Int64Slice()
	if err != nil {
		failure(c, 503, "LIMITER_UNAVAILABLE", "服务暂时不可用")
		return false
	}
	if result[0] == 0 {
		c.Header("Retry-After", strconv.FormatInt(result[1], 10))
		failure(c, 429, "RATE_LIMITED", "请求过于频繁，请稍后重试")
		return false
	}
	return true
}

func (a *App) apiRate(c *gin.Context, u User, t Token, e Endpoint, policies ...EndpointPolicy) (func(), bool) {
	p, err := a.policy()
	if err != nil {
		failure(c, 503, "POLICY_UNAVAILABLE", "限流设置暂时不可用")
		return nil, false
	}
	uKey := fmt.Sprintf("platform:v1:rate:user:%d", u.ID)
	keys := []string{uKey, fmt.Sprintf("platform:v1:rate:token:%d", t.ID), uKey + ":burst"}
	args := []any{u.RateLimit, 60, t.RateLimit, 60, p.Burst, 1}
	scopes := []string{"account_minute", "application_minute", "account_burst"}
	if len(policies) > 0 {
		keys = append(keys, uKey+":endpoint-minute:"+e.Path)
		args = append(args, policies[0].RateLimit, 60)
		scopes = append(scopes, "endpoint_minute")
	}
	if e.Cooldown > 0 {
		keys = append(keys, uKey+":endpoint:"+e.Path)
		args = append(args, 1, e.Cooldown)
		scopes = append(scopes, "endpoint_cooldown")
	}
	result, err := rateScript.Run(c.Request.Context(), a.Redis, keys, args...).Int64Slice()
	if err != nil {
		failure(c, 503, "LIMITER_UNAVAILABLE", "服务暂时不可用")
		return nil, false
	}
	c.Header("RateLimit-Limit", strconv.FormatInt(min(u.RateLimit, t.RateLimit), 10))
	if result[0] == 0 {
		if len(result) > 2 && result[2] > 0 && result[2] <= int64(len(scopes)) {
			c.Set("limitScope", scopes[result[2]-1])
		}
		c.Header("Retry-After", strconv.FormatInt(result[1], 10))
		failure(c, 429, "RATE_LIMITED", "请求过于频繁，请稍后重试")
		return nil, false
	}
	keys = []string{fmt.Sprintf("platform:v1:concurrent:%d", u.ID), "platform:v1:concurrent:global"}
	id := c.GetString("requestID")
	now := time.Now().UnixMilli()
	count, err := concurrentScript.Run(c.Request.Context(), a.Redis, keys, now, now+95000, id, p.UserConcurrency, p.GlobalConcurrency).Int()
	if err != nil {
		failure(c, 503, "LIMITER_UNAVAILABLE", "服务暂时不可用")
		return nil, false
	}
	if count == 0 {
		c.Set("limitScope", "concurrency")
		c.Header("Retry-After", "2")
		failure(c, 429, "CONCURRENCY_LIMITED", "同时进行的请求过多")
		return nil, false
	}
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		for _, key := range keys {
			_ = a.Redis.ZRem(ctx, key, id).Err()
		}
	}, true
}

func (a *App) reserve(ctx context.Context, userID, tokenID uint64, entry *Log, snapshots ...Token) (int64, error) {
	var remaining int64
	err := a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// A user lock serializes admission across all their keys and all replicas.
		var u User
		var t Token
		if err := lockForUpdate(tx).First(&u, userID).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).First(&t, tokenID).Error; err != nil {
			return err
		}
		if !u.Enabled || !t.Enabled || t.Suspended || t.RevokedAt != nil || t.UserID != u.ID || (t.ExpiresAt != nil && !t.ExpiresAt.After(time.Now())) {
			return errDisabled
		}
		if len(snapshots) > 0 {
			snapshot := snapshots[0]
			if t.KeyHash != snapshot.KeyHash || t.Scopes != snapshot.Scopes || t.AllowedIPs != snapshot.AllowedIPs {
				return errDisabled
			}
		}
		if t.Quota > 0 && t.UsedQuota >= t.Quota {
			entry.LimitScope = "application_total"
			return errQuota
		}
		day := time.Now().UTC().Format("2006-01-02")
		remaining = u.DailyQuota
		if t.Quota > 0 {
			remaining = t.Quota - t.UsedQuota - 1
		}
		buckets := []struct {
			key   string
			limit int64
		}{{fmt.Sprintf("user:%d", u.ID), u.DailyQuota}, {fmt.Sprintf("token:%d", t.ID), t.DailyQuota}}
		if _, known := endpointFor(entry.Endpoint); known {
			var policy EndpointPolicy
			if err := tx.First(&policy, "path = ?", entry.Endpoint).Error; err != nil {
				return err
			}
			if policy.Status == "disabled" || policy.Status == "maintenance" || !slices.Contains(strings.Fields(t.Scopes), policy.Scope) {
				return errEndpoint
			}
			buckets = append(buckets, struct {
				key   string
				limit int64
			}{fmt.Sprintf("endpoint:%d:%s", u.ID, digest(entry.Endpoint)[:32]), policy.DailyQuota})
		}
		for _, bucket := range buckets {
			usage := DailyUsage{Bucket: bucket.key, Day: day}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&usage).Error; err != nil {
				return err
			}
			if err := tx.Where("bucket = ? AND day = ?", bucket.key, day).First(&usage).Error; err != nil {
				return err
			}
			if usage.Requests >= bucket.limit {
				entry.LimitScope = strings.Split(bucket.key, ":")[0] + "_daily"
				return errQuota
			}
			remaining = min(remaining, bucket.limit-usage.Requests-1)
			if err := tx.Model(&usage).Update("requests", gorm.Expr("requests + 1")).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&t).Updates(map[string]any{"used_quota": gorm.Expr("used_quota + 1"), "last_used_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		entry.Admitted = true
		return tx.Create(entry).Error
	})
	if err != nil {
		entry.Admitted = false
		entry.ID = 0
	}
	return remaining, err
}
