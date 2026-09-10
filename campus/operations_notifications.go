package campus

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Gradient-Clipping/lazycampus-platform/common"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (a *App) emailAvailable() bool { return a.Config.SenderAPIKey != "" && a.Config.SenderFrom != "" }
func (a *App) notificationPreference(id uint64) (NotificationPreference, error) {
	p := defaultNotificationPreference(id)
	if err := a.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&p).Error; err != nil {
		return p, err
	}
	err := a.DB.First(&p, "user_id = ?", id).Error
	return p, err
}
func (a *App) getNotificationPreferences(c *gin.Context) {
	p, err := a.notificationPreference(currentUser(c).ID)
	if err != nil {
		failure(c, 503, "UNAVAILABLE", "无法读取通知偏好")
		return
	}
	success(c, gin.H{"preferences": p, "email_verified": p.Email != "" && p.Email == p.VerifiedEmail, "email_available": a.emailAvailable()})
}
func (a *App) updateNotificationPreferences(c *gin.Context) {
	var input NotificationPreference
	if c.ShouldBindJSON(&input) != nil {
		failure(c, 400, "INVALID_INPUT", "通知偏好无效")
		return
	}
	if input.Language == "" {
		input.Language = requestLanguage(c)
	}
	if input.Language != "en" && input.Language != "zh" {
		failure(c, 400, "INVALID_INPUT", "通知偏好无效")
		return
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	if !validEmail(input.Email) || input.QuotaPercent < 10 || input.QuotaPercent > 100 || input.ErrorPercent < 1 || input.ErrorPercent > 100 || input.ErrorMinimum < 5 || input.ErrorMinimum > 1000 || input.ExpiryDays < 1 || input.ExpiryDays > 30 {
		failure(c, 400, "INVALID_INPUT", "请检查邮箱或告警阈值")
		return
	}
	p, err := a.notificationPreference(currentUser(c).ID)
	if err != nil {
		failure(c, 503, "UNAVAILABLE", "无法读取通知偏好")
		return
	}
	err = a.DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&p, "user_id = ?", p.UserID).Error; err != nil {
			return err
		}
		if input.EmailEnabled && (!a.emailAvailable() || input.Email == "" || input.Email != p.VerifiedEmail) {
			return errDisabled
		}
		input.UserID = p.UserID
		input.VerifiedEmail = p.VerifiedEmail
		if p.Email != input.Email {
			input.VerifiedEmail = ""
			input.EmailEnabled = false
		}
		return tx.Save(&input).Error
	})
	if errors.Is(err, errDisabled) {
		failure(c, 400, "EMAIL_NOT_VERIFIED", "请先设置并验证通知邮箱")
		return
	}
	if err != nil {
		failure(c, 503, "UNAVAILABLE", "无法保存通知偏好")
		return
	}
	success(c, gin.H{"updated": true})
}

type emailChallenge struct{ Email, Hash string }

func (a *App) sendEmailVerification(c *gin.Context) {
	if !a.emailAvailable() {
		failure(c, 503, "EMAIL_UNAVAILABLE", "邮件服务暂不可用")
		return
	}
	u := currentUser(c)
	p, err := a.notificationPreference(u.ID)
	if err != nil {
		failure(c, 503, "UNAVAILABLE", "无法读取邮箱")
		return
	}
	if p.Email == "" {
		failure(c, 400, "EMAIL_REQUIRED", "请先保存通知邮箱")
		return
	}
	result, err := rateScript.Run(c.Request.Context(), a.Redis, []string{fmt.Sprintf("platform:v1:verify-email:%d:minute", u.ID), fmt.Sprintf("platform:v1:verify-email:%d:day", u.ID), "platform:v1:verify-email:target:" + digest(p.Email), "platform:v1:verify-email:global"}, 1, 60, 6, 86400, 6, 86400, 100, 86400).Int64Slice()
	if err != nil {
		failure(c, 503, "UNAVAILABLE", "邮件服务暂不可用")
		return
	}
	if result[0] == 0 {
		c.Header("Retry-After", strconv.FormatInt(result[1], 10))
		failure(c, 429, "RATE_LIMITED", "验证码发送过于频繁，请稍后重试")
		return
	}
	number, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		failure(c, 503, "UNAVAILABLE", "暂时无法发送验证码")
		return
	}
	code := fmt.Sprintf("%06d", number.Int64())
	raw, _ := common.Marshal(emailChallenge{Email: p.Email, Hash: digest(code)})
	if a.Redis.Set(c.Request.Context(), fmt.Sprintf("platform:v1:email-code:%d", u.ID), raw, 10*time.Minute).Err() != nil {
		failure(c, 503, "UNAVAILABLE", "暂时无法发送验证码")
		return
	}
	if err = a.sendMail(c.Request.Context(), p.Email, translateText(requestLanguage(c), "Lazy Campus 通知邮箱验证"), translateText(requestLanguage(c), "您的验证码是：")+code+translateText(requestLanguage(c), "\n\n有效期 10 分钟。此验证码只用于开放平台的通知邮箱验证。如非本人操作，请忽略。")); err != nil {
		failure(c, 503, "EMAIL_UNAVAILABLE", "邮件暂未确认发送，请稍后查看邮箱或重试")
		return
	}
	success(c, gin.H{"sent": true})
}
func (a *App) verifyEmail(c *gin.Context) {
	var input struct {
		Code string `json:"code"`
	}
	if c.ShouldBindJSON(&input) != nil || len(input.Code) != 6 {
		failure(c, 400, "INVALID_CODE", "请输入 6 位验证码")
		return
	}
	u := currentUser(c)
	if !a.managementRate(c, fmt.Sprintf("verify-code:%d", u.ID), 5) {
		return
	}
	key := fmt.Sprintf("platform:v1:email-code:%d", u.ID)
	raw, err := a.Redis.Get(c.Request.Context(), key).Result()
	var challenge emailChallenge
	if err != nil || common.Unmarshal([]byte(raw), &challenge) != nil || subtle.ConstantTimeCompare([]byte(challenge.Hash), []byte(digest(input.Code))) != 1 {
		failure(c, 400, "INVALID_CODE", "验证码无效或已过期")
		return
	}
	err = a.DB.Transaction(func(tx *gorm.DB) error {
		var p NotificationPreference
		if err := lockForUpdate(tx).First(&p, "user_id = ?", u.ID).Error; err != nil {
			return err
		}
		if p.Email != challenge.Email {
			return errDisabled
		}
		return tx.Model(&p).Update("verified_email", p.Email).Error
	})
	if err != nil {
		failure(c, 400, "INVALID_CODE", "邮箱已变化，请重新验证")
		return
	}
	_ = a.Redis.Del(c.Request.Context(), key).Err()
	success(c, gin.H{"verified": true})
}

var errEmailUncertain = errors.New("email delivery uncertain")
var errEmailRejected = errors.New("email rejected")
var errEmailRetry = errors.New("email retryable failure")

func (a *App) sendMail(ctx context.Context, to, subject, body string) error {
	if !a.emailAvailable() {
		return errEmailRejected
	}
	call, done := context.WithTimeout(ctx, 15*time.Second)
	defer done()
	raw, err := common.Marshal(map[string]any{"from": map[string]string{"email": a.Config.SenderFrom, "name": a.Config.SenderName}, "to": map[string]string{"email": to}, "subject": subject, "text": body})
	if err != nil {
		return errEmailRejected
	}
	req, err := http.NewRequestWithContext(call, "POST", "https://api.sender.net/v2/message/send", bytes.NewReader(raw))
	if err != nil {
		return errEmailRejected
	}
	req.Header.Set("Authorization", "Bearer "+a.Config.SenderAPIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	response, err := a.HTTP.Do(req)
	if err != nil {
		return errEmailUncertain
	}
	defer response.Body.Close()
	if response.StatusCode == 429 || response.StatusCode >= 500 {
		return errEmailRetry
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errEmailRejected
	}
	var result struct {
		Success bool `json:"success"`
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 32769))
	if err != nil || len(data) > 32768 || common.Unmarshal(data, &result) != nil {
		return errEmailUncertain
	}
	if !result.Success {
		return errEmailRejected
	}
	return nil
}
func (a *App) notifications(c *gin.Context) {
	q := a.DB.Model(&Notification{}).Where("user_id = ?", currentUser(c).ID)
	if c.Query("unread") == "true" {
		q = q.Where("read_at IS NULL AND in_app = ?", true)
	}
	if kind := c.Query("kind"); kind != "" {
		q = q.Where("kind = ?", kind)
	}
	page, size := pagination(c)
	items := []Notification{}
	var total int64
	if q.Count(&total).Error != nil || q.Order("id DESC").Offset((page-1)*size).Limit(size).Find(&items).Error != nil {
		failure(c, 503, "UNAVAILABLE", "无法读取通知")
		return
	}
	success(c, gin.H{"items": items, "total": total, "page": page, "page_size": size})
}
func (a *App) unreadNotifications(c *gin.Context) {
	var count int64
	if a.DB.Model(&Notification{}).Where("user_id = ? AND read_at IS NULL AND in_app = ?", currentUser(c).ID, true).Count(&count).Error != nil {
		failure(c, 503, "UNAVAILABLE", "无法读取通知")
		return
	}
	success(c, gin.H{"count": count})
}
func (a *App) readNotification(c *gin.Context) {
	q := a.DB.Model(&Notification{}).Where("user_id = ? AND read_at IS NULL", currentUser(c).ID)
	if c.Param("id") != "all" {
		q = q.Where("id = ?", c.Param("id"))
	}
	if q.Update("read_at", time.Now().UTC()).Error != nil {
		failure(c, 503, "UNAVAILABLE", "无法更新通知")
		return
	}
	success(c, gin.H{"updated": true})
}
func (a *App) enqueueNotice(p NotificationPreference, kind, key, title, content string) error {
	if !p.InApp && !p.EmailEnabled {
		return nil
	}
	status := "disabled"
	if p.EmailEnabled && p.Email != "" && p.Email == p.VerifiedEmail && a.emailAvailable() {
		status = "queued"
	}
	n := Notification{UserID: p.UserID, Dedupe: fmt.Sprintf("%d:%s:%s", p.UserID, kind, key), Kind: kind, Title: title, Content: content, InApp: p.InApp, EmailStatus: status, NextAttemptAt: time.Now().UTC()}
	return a.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&n).Error
}
func (a *App) applicationNotice(t Token, action, reason string) {
	p, err := a.notificationPreference(t.UserID)
	if err != nil {
		return
	}
	label := map[string]string{"suspend": "暂停", "resume": "恢复", "revoke": "撤销授权"}[action]
	_ = a.enqueueNotice(p, "application", fmt.Sprintf("%d:%s:%d", t.ID, action, time.Now().UnixNano()), "应用已"+label, fmt.Sprintf("应用「%s」已%s。\n原因：%s", t.Name, label, reason))
}
func (a *App) checkUserAlerts(u User, now time.Time) error {
	p, err := a.notificationPreference(u.ID)
	if err != nil {
		return err
	}
	if !p.InApp && !p.EmailEnabled {
		return nil
	}
	day := now.Format("2006-01-02")
	hour := now.Format("2006-01-02T15")
	if p.QuotaEnabled {
		var usage DailyUsage
		err := a.DB.First(&usage, "bucket = ? AND day = ?", fmt.Sprintf("user:%d", u.ID), day).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if usage.Requests*100 >= u.DailyQuota*int64(p.QuotaPercent) {
			if err = a.enqueueNotice(p, "quota", day, "今日额度提醒", fmt.Sprintf("今日已用 %d / %d 次，达到您设置的 %d%% 阈值。", usage.Requests, u.DailyQuota, p.QuotaPercent)); err != nil {
				return err
			}
		}
		var apps []Token
		if err = a.DB.Where("user_id = ? AND enabled = ? AND suspended = ? AND revoked_at IS NULL AND quota > 0 AND used_quota * 100 >= quota * ?", u.ID, true, false, p.QuotaPercent).Find(&apps).Error; err != nil {
			return err
		}
		for _, app := range apps {
			if err = a.enqueueNotice(p, "quota", fmt.Sprintf("app:%d:%d", app.ID, app.Quota), "应用总额度提醒", fmt.Sprintf("应用「%s」已用 %d / %d 次，达到您设置的 %d%% 阈值。", app.Name, app.UsedQuota, app.Quota, p.QuotaPercent)); err != nil {
				return err
			}
		}
	}
	if p.ErrorsEnabled || p.RateEnabled {
		var m Metrics
		if err = a.DB.Model(&Log{}).Where("user_id = ? AND created_at >= ?", u.ID, now.Add(-15*time.Minute)).Select(metricSQL).Scan(&m).Error; err != nil {
			return err
		}
		if p.ErrorsEnabled && m.Requests >= int64(p.ErrorMinimum) && m.Errors*100 >= m.Requests*int64(p.ErrorPercent) {
			if err = a.enqueueNotice(p, "errors", hour, "调用错误率升高", fmt.Sprintf("最近 15 分钟 %d 次请求中有 %d 次失败。请在调用日志中查看错误码和请求详情。", m.Requests, m.Errors)); err != nil {
				return err
			}
		}
		if p.RateEnabled && m.Limited > 0 {
			if err = a.enqueueNotice(p, "rate", hour, "调用触发限流", fmt.Sprintf("最近 15 分钟有 %d 次请求被额度或速率限制。请按 Retry-After 重试或调整调用频率。", m.Limited)); err != nil {
				return err
			}
		}
	}
	if p.ExpiryEnabled {
		tokens := []Token{}
		if err = a.DB.Where("user_id = ? AND enabled = ? AND suspended = ? AND revoked_at IS NULL AND expires_at > ? AND expires_at <= ?", u.ID, true, false, now, now.Add(time.Duration(p.ExpiryDays)*24*time.Hour)).Find(&tokens).Error; err != nil {
			return err
		}
		for _, t := range tokens {
			if err = a.enqueueNotice(p, "expiry", fmt.Sprintf("%d:%d:%s", t.ID, t.ExpiresAt.Unix(), day), "应用即将到期", fmt.Sprintf("应用「%s」将在 %d 天内到期，请及时调整有效期。", t.Name, p.ExpiryDays)); err != nil {
				return err
			}
		}
	}
	return nil
}
func (a *App) deliverNotifications(ctx context.Context) {
	now := time.Now().UTC()
	// A crash after sending must never silently deliver the same email again.
	_ = a.DB.Model(&Notification{}).Where("email_status = ? AND next_attempt_at < ?", "sending", now.Add(-3*time.Minute)).Update("email_status", "uncertain").Error
	items := []Notification{}
	if a.DB.Where("email_status IN ? AND next_attempt_at <= ?", []string{"queued", "retry"}, now).Order("id").Limit(10).Find(&items).Error != nil {
		return
	}
	for _, n := range items {
		if ctx.Err() != nil {
			return
		}
		claim := a.DB.Model(&n).Where("email_status IN ?", []string{"queued", "retry"}).Updates(map[string]any{"email_status": "sending", "email_attempts": gorm.Expr("email_attempts + 1"), "next_attempt_at": now})
		if claim.Error != nil || claim.RowsAffected != 1 {
			continue
		}
		p, err := a.notificationPreference(n.UserID)
		status := "disabled"
		if err != nil {
			status = "retry"
		} else if p.EmailEnabled && p.Email != "" && p.Email == p.VerifiedEmail && a.emailAvailable() {
			limits, e := rateScript.Run(ctx, a.Redis, []string{fmt.Sprintf("platform:v1:alert-email:%d:hour", n.UserID), fmt.Sprintf("platform:v1:alert-email:%d:day", n.UserID), "platform:v1:alert-email:global"}, 3, 3600, 10, 86400, 200, 86400).Int64Slice()
			if e != nil {
				status = "retry"
			} else if limits[0] == 0 {
				status = "throttled"
			} else {
				e = a.sendMail(ctx, p.Email, translateText(p.Language, n.Title), translateText(p.Language, n.Content)+translateText(p.Language, "\n\n查看通知：")+a.Config.Origin+translateText(p.Language, "/notifications\n通知偏好：")+a.Config.Origin+"/profile#notifications")
				switch {
				case e == nil:
					status = "sent"
				case errors.Is(e, errEmailUncertain):
					status = "uncertain"
				case errors.Is(e, errEmailRetry):
					status = "retry"
				default:
					status = "failed"
				}
			}
		}
		if status == "retry" && n.EmailAttempts+1 >= 5 {
			status = "failed"
		}
		_ = a.DB.Model(&n).Updates(map[string]any{"email_status": status, "next_attempt_at": time.Now().UTC().Add(time.Duration(1<<min(n.EmailAttempts, 5)) * time.Minute)}).Error
	}
}
func (a *App) operationsWorker(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		lease := randomKey(16)
		locked, err := a.Redis.SetNX(ctx, "platform:v1:operations-worker", lease, 5*time.Minute).Result()
		if err != nil || !locked {
			continue
		}
		func() {
			work, done := context.WithTimeout(ctx, 4*time.Minute)
			defer done()
			defer func() {
				_ = a.Redis.Eval(context.Background(), `if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) end return 0`, []string{"platform:v1:operations-worker"}, lease).Err()
			}()
			var after uint64
			for {
				users := []User{}
				if a.DB.WithContext(work).Where("enabled = ? AND id > ?", true, after).Order("id").Limit(100).Find(&users).Error != nil {
					break
				}
				if len(users) == 0 {
					break
				}
				for _, u := range users {
					if work.Err() != nil {
						return
					}
					if a.checkUserAlerts(u, time.Now().UTC()) != nil {
						log.Print("notification evaluation failed")
					}
					after = u.ID
				}
			}
			a.deliverNotifications(work)
			a.retryRevokedSessions(work)
		}()
	}
}
