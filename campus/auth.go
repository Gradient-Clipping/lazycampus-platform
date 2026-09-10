package campus

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Gradient-Clipping/lazycampus-platform/common"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const sessionCookie = "__Host-platform_session"
const flowCookie = "__Host-platform_oidc"

var opaqueID = regexp.MustCompile(`^[a-zA-Z0-9_-]{20,128}$`)
var identityID = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)

type Claims struct {
	Subject       string `json:"sub"`
	Username      string `json:"preferred_username"`
	Name          string `json:"name"`
	IdentityID    string `json:"identity_id"`
	StudentNumber string `json:"student_number"`
	SID           string `json:"sid"`
	RealmAccess   struct {
		Roles []string `json:"roles"`
	} `json:"realm_access"`
}
type loginTransaction struct{ Verifier, Nonce, BrowserHash, Mode, ReturnTo string }
type browserSession struct {
	UserID                                    uint64
	Subject, SID, CSRF, IDToken, RefreshToken string
	ExpiresAt, RefreshAt                      int64
}

func AuthorizedIdentity(claims Claims) (User, error) {
	if claims.Subject == "" || len(claims.Subject) > 128 || len(claims.Username) > 128 {
		return User{}, errors.New("missing identity")
	}
	role := "user"
	if claims.Username == "ystemsrx" && slices.Contains(claims.RealmAccess.Roles, "platform-admin") {
		role = "admin"
	} else if !identityID.MatchString(claims.IdentityID) || claims.StudentNumber == "" {
		return User{}, errors.New("school identity required")
	}
	return User{Subject: claims.Subject, IdentityID: claims.IdentityID, Username: claims.Username, DisplayName: claims.Name, Role: role, Enabled: true, DailyQuota: DefaultDailyQuota, RateLimit: DefaultRateLimit, LastLoginAt: time.Now().UTC()}, nil
}

func (a *App) cookie(c *gin.Context, name, value string, seconds int) {
	http.SetCookie(c.Writer, &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: a.Config.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: seconds})
}

func (a *App) login(c *gin.Context) {
	if !a.managementRate(c, "login:"+digest(c.ClientIP()), 20) {
		return
	}
	mode := c.Query("mode")
	if mode != "campus" && mode != "admin" {
		mode = "silent"
	}
	returnTo := c.Query("return_to")
	if !strings.HasPrefix(returnTo, "/") || strings.HasPrefix(returnTo, "//") || strings.ContainsAny(returnTo, "\\\r\n") || strings.HasPrefix(returnTo, "/auth/") {
		returnTo = "/"
	}
	state, browser := randomKey(32), randomKey(32)
	tx := loginTransaction{Verifier: oauth2.GenerateVerifier(), Nonce: randomKey(32), BrowserHash: digest(browser), Mode: mode, ReturnTo: returnTo}
	data, _ := common.Marshal(tx)
	if err := a.Redis.Set(c.Request.Context(), "platform:v1:oidc:"+digest(state), data, 5*time.Minute).Err(); err != nil {
		failure(c, 503, "SSO_UNAVAILABLE", "统一登录暂时不可用")
		return
	}
	a.cookie(c, flowCookie, browser, 300)
	opts := []oauth2.AuthCodeOption{oidc.Nonce(tx.Nonce), oauth2.S256ChallengeOption(tx.Verifier)}
	if mode == "silent" {
		opts = append(opts, oauth2.SetAuthURLParam("prompt", "none"))
	}
	if mode == "campus" {
		opts = append(opts, oauth2.SetAuthURLParam("kc_idp_hint", "campus"))
	}
	if mode == "admin" {
		opts = append(opts, oauth2.SetAuthURLParam("kc_idp_hint", ""))
	}
	c.Redirect(302, a.OAuth.AuthCodeURL(state, opts...))
}

func (a *App) callback(c *gin.Context) {
	state := c.Query("state")
	browser, _ := c.Cookie(flowCookie)
	if !opaqueID.MatchString(state) || !opaqueID.MatchString(browser) {
		a.authError(c, "expired")
		return
	}
	key := "platform:v1:oidc:" + digest(state)
	raw, err := a.Redis.Get(c.Request.Context(), key).Result()
	var tx loginTransaction
	if err != nil || common.Unmarshal([]byte(raw), &tx) != nil || subtle.ConstantTimeCompare([]byte(tx.BrowserHash), []byte(digest(browser))) != 1 {
		a.authError(c, "expired")
		return
	}
	consumed, err := a.Redis.GetDel(c.Request.Context(), key).Result()
	if err != nil || consumed != raw {
		a.authError(c, "expired")
		return
	}
	a.cookie(c, flowCookie, "", -1)
	if code := c.Query("error"); code != "" {
		if tx.Mode == "silent" && slices.Contains([]string{"login_required", "interaction_required", "consent_required", "account_selection_required"}, code) {
			c.Redirect(302, "/auth/start?mode=campus&return_to="+url.QueryEscape(tx.ReturnTo))
			return
		}
		a.authError(c, "denied")
		return
	}
	ctx := oidc.ClientContext(c.Request.Context(), a.HTTP)
	tokens, err := a.OAuth.Exchange(ctx, c.Query("code"), oauth2.VerifierOption(tx.Verifier))
	if err != nil {
		a.authError(c, "expired")
		return
	}
	rawID, _ := tokens.Extra("id_token").(string)
	id, err := a.Provider.Verifier(&oidc.Config{ClientID: a.Config.ClientID, SupportedSigningAlgs: []string{"RS256"}}).Verify(ctx, rawID)
	if err != nil || subtle.ConstantTimeCompare([]byte(id.Nonce), []byte(tx.Nonce)) != 1 {
		a.authError(c, "denied")
		return
	}
	var claims Claims
	if id.Claims(&claims) != nil {
		a.authError(c, "denied")
		return
	}
	user, err := AuthorizedIdentity(claims)
	if err != nil {
		a.authError(c, "denied")
		return
	}
	policy, err := a.policy()
	if err != nil {
		a.authError(c, "unavailable")
		return
	}
	user.DailyQuota, user.RateLimit, user.InheritLimits = policy.DailyQuota, policy.RateLimit, true
	err = a.DB.Transaction(func(db *gorm.DB) error {
		if err := db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "subject"}}, DoNothing: true}).Create(&user).Error; err != nil {
			return err
		}
		var existing User
		if err := lockForUpdate(db).Where("subject = ?", user.Subject).First(&existing).Error; err != nil {
			return err
		}
		if !existing.Enabled {
			return errors.New("disabled")
		}
		if err := db.Model(&existing).Updates(map[string]any{"username": user.Username, "display_name": user.DisplayName, "identity_id": user.IdentityID, "role": user.Role, "last_login_at": user.LastLoginAt}).Error; err != nil {
			return err
		}
		user.ID = existing.ID
		return db.Create(&Audit{ActorID: user.ID, Action: "sso.login", Target: user.Username}).Error
	})
	if err != nil {
		a.authError(c, "denied")
		return
	}
	s := browserSession{UserID: user.ID, Subject: claims.Subject, SID: claims.SID, CSRF: randomKey(32), IDToken: rawID, RefreshToken: tokens.RefreshToken, ExpiresAt: time.Now().Add(8 * time.Hour).Unix(), RefreshAt: time.Now().Add(4 * time.Minute).Unix()}
	sessionID := randomKey(32)
	sessionKey := "platform:v1:session:" + digest(sessionID)
	data, _ := common.Marshal(s)
	if err = a.Redis.Set(ctx, sessionKey, data, 30*time.Minute).Err(); err != nil {
		a.authError(c, "unavailable")
		return
	}
	// Index only hashes, for signed Keycloak back-channel session revocation.
	pipe := a.Redis.TxPipeline()
	pipe.SAdd(ctx, "platform:v1:sessions:"+digest(claims.Subject), sessionKey)
	pipe.Expire(ctx, "platform:v1:sessions:"+digest(claims.Subject), 8*time.Hour)
	if _, err = pipe.Exec(ctx); err != nil {
		_ = a.Redis.Del(ctx, sessionKey).Err()
		a.authError(c, "unavailable")
		return
	}
	if err = a.registerDevice(c, sessionKey, s); err != nil {
		_ = a.Redis.Del(ctx, sessionKey).Err()
		a.authError(c, "unavailable")
		return
	}
	if old, _ := c.Cookie(sessionCookie); opaqueID.MatchString(old) {
		_ = a.Redis.Del(ctx, "platform:v1:session:"+digest(old)).Err()
	}
	a.cookie(c, sessionCookie, sessionID, 8*3600)
	c.Redirect(302, tx.ReturnTo)
}

func (a *App) sessionAuth(c *gin.Context) {
	id, _ := c.Cookie(sessionCookie)
	if !opaqueID.MatchString(id) {
		failure(c, 401, "SSO_REQUIRED", "请重新打开页面登录")
		return
	}
	key := "platform:v1:session:" + digest(id)
	raw, err := a.Redis.Get(c.Request.Context(), key).Result()
	var session browserSession
	if err != nil || common.Unmarshal([]byte(raw), &session) != nil || session.ExpiresAt <= time.Now().Unix() {
		failure(c, 401, "SSO_REQUIRED", "登录已过期")
		return
	}
	var user User
	if a.DB.First(&user, session.UserID).Error != nil || !user.Enabled {
		failure(c, 403, "ACCOUNT_DISABLED", "当前账号不可用")
		return
	}
	if err = a.registerDevice(c, key, session); err != nil {
		if errors.Is(err, errDisabled) {
			failure(c, 401, "SESSION_REVOKED", "此设备已退出登录")
		} else {
			failure(c, 503, "UNAVAILABLE", "无法检查登录设备")
		}
		return
	}
	if session.RefreshAt <= time.Now().Unix() {
		// Refresh-token rotation is serialized across tabs and application replicas.
		locked, lockErr := a.Redis.SetNX(c.Request.Context(), key+":refresh", 1, 30*time.Second).Result()
		if lockErr != nil {
			failure(c, 503, "SSO_UNAVAILABLE", "统一登录暂时不可用")
			return
		}
		if locked {
			defer a.Redis.Del(c.Request.Context(), key+":refresh")
			if !a.refreshSession(c, key, &session, &user) {
				return
			}
		} else {
			c.Header("Retry-After", "1")
			failure(c, 503, "SSO_REFRESHING", "登录正在更新，请稍后重试")
			return
		}
	}
	if c.Request.Method != "GET" {
		if c.GetHeader("Origin") != a.Config.Origin || subtle.ConstantTimeCompare([]byte(c.GetHeader("X-CSRF-Token")), []byte(session.CSRF)) != 1 {
			failure(c, 403, "CSRF_REJECTED", "请刷新页面后重试")
			return
		}
	}
	if !a.managementRate(c, "console:"+digest(user.Subject), 120) {
		return
	}
	if err = a.Redis.Expire(c.Request.Context(), key, min(30*time.Minute, time.Until(time.Unix(session.ExpiresAt, 0)))).Err(); err != nil {
		failure(c, 503, "SSO_UNAVAILABLE", "登录服务暂时不可用")
		return
	}
	c.Set("user", user)
	c.Set("session", session)
	c.Set("sessionKey", key)
	c.Next()
}

func (a *App) refreshSession(c *gin.Context, key string, s *browserSession, user *User) bool {
	ctx := oidc.ClientContext(c.Request.Context(), a.HTTP)
	tokens, err := a.OAuth.TokenSource(ctx, &oauth2.Token{RefreshToken: s.RefreshToken, Expiry: time.Now().Add(-time.Hour)}).Token()
	if err != nil {
		_ = a.Redis.Del(ctx, key).Err()
		failure(c, 401, "SSO_REQUIRED", "登录已过期")
		return false
	}
	rawID, _ := tokens.Extra("id_token").(string)
	id, err := a.Provider.Verifier(&oidc.Config{ClientID: a.Config.ClientID, SupportedSigningAlgs: []string{"RS256"}}).Verify(ctx, rawID)
	var claims Claims
	if err != nil || id.Claims(&claims) != nil || claims.Subject != s.Subject {
		_ = a.Redis.Del(ctx, key).Err()
		failure(c, 401, "SSO_REQUIRED", "登录已过期")
		return false
	}
	identity, err := AuthorizedIdentity(claims)
	if err != nil {
		_ = a.Redis.Del(ctx, key).Err()
		failure(c, 403, "ACCESS_DENIED", "当前身份没有访问权限")
		return false
	}
	if err = a.DB.Model(user).Updates(map[string]any{"role": identity.Role, "identity_id": identity.IdentityID}).Error; err != nil {
		failure(c, 503, "UNAVAILABLE", "服务暂时不可用")
		return false
	}
	user.Role = identity.Role
	user.IdentityID = identity.IdentityID
	s.RefreshToken = tokens.RefreshToken
	s.IDToken = rawID
	s.RefreshAt = time.Now().Add(4 * time.Minute).Unix()
	data, _ := common.Marshal(s)
	// Never resurrect a session revoked while the refresh request was in flight.
	saved, err := a.Redis.SetXX(ctx, key, data, min(30*time.Minute, time.Until(time.Unix(s.ExpiresAt, 0)))).Result()
	if err != nil || !saved {
		failure(c, 401, "SSO_REQUIRED", "登录已过期")
		return false
	}
	return true
}

func (a *App) logout(c *gin.Context) {
	s := c.MustGet("session").(browserSession)
	if a.DB.Model(&SessionDevice{}).Where("id = ? AND user_id = ?", strings.TrimPrefix(c.GetString("sessionKey"), "platform:v1:session:"), s.UserID).Update("revoked_at", time.Now().UTC()).Error != nil {
		failure(c, 503, "UNAVAILABLE", "无法退出登录")
		return
	}
	if err := a.Redis.Del(c.Request.Context(), c.GetString("sessionKey")).Err(); err != nil {
		failure(c, 503, "UNAVAILABLE", "请稍后重试")
		return
	}
	a.cookie(c, sessionCookie, "", -1)
	u := a.Config.Issuer + "/protocol/openid-connect/logout?" + url.Values{"id_token_hint": {s.IDToken}, "post_logout_redirect_uri": {a.Config.Origin + "/signed-out"}}.Encode()
	success(c, gin.H{"url": u})
}

func (a *App) authError(c *gin.Context, code string) {
	c.Redirect(302, "/auth-error?reason="+url.QueryEscape(code))
}

func (a *App) backchannelLogout(c *gin.Context) {
	raw := c.PostForm("logout_token")
	ctx := oidc.ClientContext(c.Request.Context(), a.HTTP)
	jwt, err := a.Provider.Verifier(&oidc.Config{ClientID: a.Config.ClientID, SupportedSigningAlgs: []string{"RS256"}}).Verify(ctx, raw)
	var claims struct {
		Subject  string         `json:"sub"`
		SID      string         `json:"sid"`
		JTI      string         `json:"jti"`
		IssuedAt int64          `json:"iat"`
		Nonce    string         `json:"nonce"`
		Events   map[string]any `json:"events"`
	}
	if err != nil || jwt.Claims(&claims) != nil || claims.JTI == "" || claims.Nonce != "" || time.Now().Unix()-claims.IssuedAt > 300 || claims.IssuedAt > time.Now().Unix()+30 {
		failure(c, 400, "INVALID_LOGOUT", "无效请求")
		return
	}
	if _, ok := claims.Events["http://schemas.openid.net/event/backchannel-logout"]; !ok || claims.Subject == "" {
		failure(c, 400, "INVALID_LOGOUT", "无效请求")
		return
	}
	logoutKey := "platform:v1:logout:" + digest(claims.JTI)
	seen, err := a.Redis.SetNX(ctx, logoutKey, 1, 10*time.Minute).Result()
	if err != nil {
		failure(c, 503, "UNAVAILABLE", "服务暂时不可用")
		return
	}
	if !seen {
		c.Status(200)
		return
	}
	completed := false
	defer func() {
		if !completed {
			_ = a.Redis.Del(ctx, logoutKey).Err()
		}
	}()
	index := "platform:v1:sessions:" + digest(claims.Subject)
	keys, err := a.Redis.SMembers(ctx, index).Result()
	if err != nil {
		failure(c, 503, "UNAVAILABLE", "服务暂时不可用")
		return
	}
	for _, key := range keys {
		raw, err := a.Redis.Get(ctx, key).Result()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			failure(c, 503, "UNAVAILABLE", "服务暂时不可用")
			return
		}
		var s browserSession
		if common.Unmarshal([]byte(raw), &s) == nil && (claims.SID == "" || s.SID == claims.SID) {
			if a.DB.Model(&SessionDevice{}).Where("id = ? AND user_id = ?", strings.TrimPrefix(key, "platform:v1:session:"), s.UserID).Update("revoked_at", time.Now().UTC()).Error != nil {
				failure(c, 503, "UNAVAILABLE", "服务暂时不可用")
				return
			}
			if a.Redis.Del(ctx, key).Err() != nil {
				failure(c, 503, "UNAVAILABLE", "服务暂时不可用")
				return
			}
			_ = a.Redis.SRem(ctx, index, key).Err()
		}
	}
	completed = true
	c.Status(200)
}
