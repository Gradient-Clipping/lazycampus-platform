package campus

import (
	"context"
	"errors"
	"github.com/Gradient-Clipping/lazycampus-platform/common"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm/clause"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func deviceName(ua string) string {
	browser := "浏览器"
	os := "未知系统"
	switch {
	case strings.Contains(ua, "Edg/"):
		browser = "Edge"
	case strings.Contains(ua, "Firefox/"):
		browser = "Firefox"
	case strings.Contains(ua, "Chrome/") || strings.Contains(ua, "CriOS/"):
		browser = "Chrome"
	case strings.Contains(ua, "Safari/"):
		browser = "Safari"
	}
	switch {
	case strings.Contains(ua, "Android"):
		os = "Android"
	case strings.Contains(ua, "iPhone") || strings.Contains(ua, "iPad"):
		os = "iOS"
	case strings.Contains(ua, "Windows"):
		os = "Windows"
	case strings.Contains(ua, "Macintosh"):
		os = "macOS"
	case strings.Contains(ua, "Linux"):
		os = "Linux"
	}
	return browser + " · " + os
}
func (a *App) registerDevice(c *gin.Context, key string, s browserSession) error {
	now := time.Now().UTC()
	row := SessionDevice{ID: strings.TrimPrefix(key, "platform:v1:session:"), UserID: s.UserID, Device: deviceName(c.Request.UserAgent()), IP: c.ClientIP(), CreatedAt: now, LastActiveAt: now, ExpiresAt: time.Unix(s.ExpiresAt, 0).UTC()}
	if err := a.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return err
	}
	if err := a.DB.First(&row, "id = ?", row.ID).Error; err != nil {
		return err
	}
	if row.UserID != s.UserID || row.RevokedAt != nil || !row.ExpiresAt.After(now) {
		return errDisabled
	}
	// Session polling is not a human visit or device activity.
	if c.FullPath() != "/api/me" && c.FullPath() != "/api/notifications/unread" && c.FullPath() != "/api/notifications" && c.FullPath() != "/api/sessions" {
		return a.DB.Model(&row).Where("last_active_at < ? AND revoked_at IS NULL", now.Add(-time.Minute)).Updates(map[string]any{"last_active_at": now, "ip": c.ClientIP()}).Error
	}
	return nil
}
func (a *App) sessions(c *gin.Context) {
	items := []SessionDevice{}
	if a.DB.Where("user_id = ? AND revoked_at IS NULL AND expires_at > ?", currentUser(c).ID, time.Now().UTC()).Order("last_active_at DESC").Limit(100).Find(&items).Error != nil {
		failure(c, 503, "UNAVAILABLE", "无法读取登录设备")
		return
	}
	active := []SessionDevice{}
	for _, item := range items {
		exists, err := a.Redis.Exists(c.Request.Context(), "platform:v1:session:"+item.ID).Result()
		if err != nil {
			failure(c, 503, "UNAVAILABLE", "无法读取会话状态")
			return
		}
		if exists > 0 {
			active = append(active, item)
		}
	}
	success(c, gin.H{"items": active, "current_id": strings.TrimPrefix(c.GetString("sessionKey"), "platform:v1:session:")})
}
func (a *App) revokeSession(c *gin.Context) {
	id := c.Param("id")
	if len(id) != 64 {
		failure(c, 404, "NOT_FOUND", "会话不存在")
		return
	}
	var device SessionDevice
	if a.DB.Where("id = ? AND user_id = ?", id, currentUser(c).ID).First(&device).Error != nil {
		failure(c, 404, "NOT_FOUND", "会话不存在")
		return
	}
	current := c.MustGet("session").(browserSession)
	provider, err := a.revokeDevice(c.Request.Context(), device, current.SID, c.GetString("sessionKey") == "platform:v1:session:"+id)
	if err != nil {
		failure(c, 503, "UNAVAILABLE", "无法退出设备，请重试")
		return
	}
	_ = a.DB.Create(&Audit{ActorID: current.UserID, Action: "session.revoke", Target: id}).Error
	isCurrent := strings.HasSuffix(c.GetString("sessionKey"), id)
	if isCurrent {
		a.cookie(c, sessionCookie, "", -1)
	}
	success(c, gin.H{"revoked": true, "current": isCurrent, "sso_revoked": provider})
}
func (a *App) revokeOtherSessions(c *gin.Context) {
	current := c.MustGet("session").(browserSession)
	currentID := strings.TrimPrefix(c.GetString("sessionKey"), "platform:v1:session:")
	items := []SessionDevice{}
	if a.DB.Where("user_id = ? AND id <> ? AND revoked_at IS NULL AND expires_at > ?", current.UserID, currentID, time.Now().UTC()).Find(&items).Error != nil {
		failure(c, 503, "UNAVAILABLE", "无法读取登录设备")
		return
	}
	provider := true
	for _, item := range items {
		ok, err := a.revokeDevice(c.Request.Context(), item, current.SID, false)
		if err != nil {
			failure(c, 503, "UNAVAILABLE", "部分设备未退出，请刷新后重试")
			return
		}
		provider = provider && ok
	}
	_ = a.DB.Create(&Audit{ActorID: current.UserID, Action: "session.revoke_others", Target: currentID}).Error
	success(c, gin.H{"revoked": len(items), "sso_revoked": provider})
}
func (a *App) revokeDevice(ctx context.Context, device SessionDevice, currentSID string, isCurrent bool) (bool, error) {
	if err := a.DB.WithContext(ctx).Model(&device).Update("revoked_at", time.Now().UTC()).Error; err != nil {
		return false, err
	}
	key := "platform:v1:session:" + device.ID
	raw, err := a.Redis.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	var s browserSession
	if common.Unmarshal([]byte(raw), &s) != nil || s.UserID != device.UserID {
		return false, errors.New("session mismatch")
	}
	// A single Keycloak browser session can own several platform tabs/cookies.
	// Do not terminate the current browser's SSO while revoking another local grant.
	if !isCurrent && s.SID != "" && s.SID == currentSID {
		return true, a.Redis.Del(ctx, key).Err()
	}
	if s.RefreshToken != "" && !a.logoutProvider(ctx, s) {
		return false, nil
	}
	return true, a.Redis.Del(ctx, key).Err()
}
func (a *App) logoutProvider(ctx context.Context, s browserSession) bool {
	call, done := context.WithTimeout(ctx, 10*time.Second)
	defer done()
	values := url.Values{"client_id": {a.Config.ClientID}, "client_secret": {a.Config.ClientSecret}, "refresh_token": {s.RefreshToken}}
	req, err := http.NewRequestWithContext(call, "POST", a.Config.Issuer+"/protocol/openid-connect/logout", strings.NewReader(values.Encode()))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := a.HTTP.Do(req)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode >= 200 && response.StatusCode < 300
}
func (a *App) retryRevokedSessions(ctx context.Context) {
	items := []SessionDevice{}
	if a.DB.WithContext(ctx).Where("revoked_at IS NOT NULL AND expires_at > ?", time.Now().UTC()).Limit(100).Find(&items).Error != nil {
		return
	}
	for _, item := range items {
		if ctx.Err() != nil {
			return
		}
		_, _ = a.revokeDevice(ctx, item, "", false)
	}
}
