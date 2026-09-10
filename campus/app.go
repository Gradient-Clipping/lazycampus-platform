package campus

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

type App struct {
	Config   Config
	DB       *gorm.DB
	Redis    *redis.Client
	HTTP     *http.Client
	Provider *oidc.Provider
	OAuth    oauth2.Config
	cancel   context.CancelFunc
}

func New(ctx context.Context, config Config) (*App, error) {
	db, err := OpenDatabase(config.DatabaseDSN)
	if err != nil {
		return nil, err
	}
	pool, _ := db.DB()
	options, err := redis.ParseURL(config.RedisURL)
	if err != nil {
		pool.Close()
		return nil, errors.New("invalid Redis configuration")
	}
	r := redis.NewClient(options)
	httpClient := &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	app := &App{Config: config, DB: db, Redis: r, HTTP: httpClient}
	startup, done := context.WithTimeout(ctx, 30*time.Second)
	defer done()
	if r.Ping(startup).Err() != nil {
		app.Close()
		return nil, errors.New("Redis unavailable")
	}
	if err = Migrate(db); err != nil {
		app.Close()
		return nil, errors.New("database migration failed")
	}
	app.Provider, err = oidc.NewProvider(oidc.ClientContext(startup, httpClient), config.Issuer)
	if err != nil {
		app.Close()
		return nil, errors.New("OIDC discovery failed")
	}
	app.OAuth = oauth2.Config{ClientID: config.ClientID, ClientSecret: config.ClientSecret, Endpoint: app.Provider.Endpoint(), RedirectURL: config.Origin + "/auth/callback", Scopes: []string{"openid", "profile", "roles"}}
	background, cancel := context.WithCancel(ctx)
	app.cancel = cancel
	go app.retention(background)
	go app.operationsWorker(background)
	return app, nil
}

func (a *App) Close() {
	if a.cancel != nil {
		a.cancel()
	}
	if a.Redis != nil {
		_ = a.Redis.Close()
	}
	if a.DB != nil {
		if db, err := a.DB.DB(); err == nil {
			_ = db.Close()
		}
	}
}

func (a *App) Router() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, _ any) { failure(c, 500, "INTERNAL_ERROR", "服务暂时不可用") }))
	_ = r.SetTrustedProxies([]string{"127.0.0.1", "::1", "10.6.0.3/32", "10.42.0.0/16"})
	r.Use(func(c *gin.Context) {
		c.Set("requestID", randomKey(18))
		c.Header("X-Request-Id", c.GetString("requestID"))
		c.Header("Cache-Control", "no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		if a.Config.SecureCookies {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32*1024)
		c.Next()
	})
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok", "revision": a.Config.Revision}) })
	r.GET("/readyz", a.ready)
	r.GET("/auth/start", a.login)
	r.GET("/auth/callback", a.callback)
	r.POST("/auth/backchannel-logout", a.backchannelLogout)
	r.GET("/api/status", func(c *gin.Context) {
		success(c, gin.H{"name": "Lazy Campus 开放平台", "revision": a.Config.Revision, "source": "https://github.com/Gradient-Clipping/lazycampus-platform", "sso_only": true})
	})
	r.GET("/openapi.json", a.openAPI)
	r.GET("/api/announcements", a.announcements)
	r.GET("/api/site", a.publicSite)
	api := r.Group("/api", a.sessionAuth)
	api.GET("/me", a.me)
	api.POST("/visit", a.visit)
	api.GET("/catalog", a.catalog)
	api.GET("/dashboard", a.dashboard)
	api.GET("/analytics", a.analytics)
	api.GET("/tokens", a.listTokens)
	api.POST("/tokens", a.createToken)
	api.PATCH("/tokens/:id", a.updateToken)
	api.POST("/tokens/:id/rotate", a.rotateToken)
	api.DELETE("/tokens/:id", a.deleteToken)
	api.GET("/logs", a.logs)
	api.GET("/logs/:id", a.logDetail)
	api.GET("/sessions", a.sessions)
	api.DELETE("/sessions/:id", a.revokeSession)
	api.POST("/sessions/revoke-others", a.revokeOtherSessions)
	api.GET("/notification-preferences", a.getNotificationPreferences)
	api.PUT("/notification-preferences", a.updateNotificationPreferences)
	api.POST("/notification-preferences/send-verification", a.sendEmailVerification)
	api.POST("/notification-preferences/verify", a.verifyEmail)
	api.GET("/notifications", a.notifications)
	api.GET("/notifications/unread", a.unreadNotifications)
	api.POST("/notifications/:id/read", a.readNotification)
	api.GET("/notice", a.notice)
	api.POST("/logout", a.logout)
	admin := api.Group("/admin", func(c *gin.Context) {
		if currentUser(c).Role != "admin" {
			failure(c, 403, "ADMIN_REQUIRED", "需要管理员权限")
			return
		}
		c.Next()
	})
	admin.GET("/users", a.listUsers)
	admin.PATCH("/users/:id", a.updateUser)
	admin.GET("/logs", a.logs)
	admin.GET("/logs/:id", a.logDetail)
	admin.GET("/analytics", a.analytics)
	admin.GET("/tokens", a.adminTokens)
	admin.PATCH("/tokens/:id", a.regulateToken)
	admin.GET("/endpoints", a.catalog)
	admin.PUT("/endpoints", a.updateEndpoint)
	admin.GET("/settings/policy", a.getPolicy)
	admin.PUT("/settings/policy", a.updatePolicy)
	admin.GET("/settings/site", a.getSite)
	admin.PUT("/settings/site", a.updateSite)
	admin.GET("/audits", a.audits)
	admin.GET("/system", a.system)
	admin.PUT("/notice", a.updateNotice)
	admin.DELETE("/announcements/:id", a.deleteAnnouncement)
	r.GET("/v1/*path", a.gateway)
	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		if c.Request.Method != "GET" || strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/v1/") || strings.HasPrefix(p, "/auth/") || strings.HasPrefix(p, "/internal/") {
			failure(c, 404, "NOT_FOUND", "接口不存在")
			return
		}
		clean := filepath.Clean(filepath.Join(a.Config.StaticDir, filepath.FromSlash(p)))
		abs, _ := filepath.Abs(clean)
		root, _ := filepath.Abs(a.Config.StaticDir)
		if strings.HasPrefix(abs, root+string(os.PathSeparator)) {
			if info, err := os.Stat(abs); err == nil && !info.IsDir() {
				if strings.HasPrefix(p, "/static/") {
					c.Header("Cache-Control", "public, max-age=31536000, immutable")
				}
				c.File(abs)
				return
			}
		}
		if strings.Contains(filepath.Base(p), ".") {
			failure(c, 404, "NOT_FOUND", "文件不存在")
			return
		}
		c.File(filepath.Join(a.Config.StaticDir, "index.html"))
	})
	return r
}

func (a *App) ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	pool, err := a.DB.DB()
	if err != nil || pool.PingContext(ctx) != nil || a.Redis.Ping(ctx).Err() != nil {
		failure(c, 503, "UNAVAILABLE", "服务暂时不可用")
		return
	}
	c.JSON(200, gin.H{"status": "ready", "revision": a.Config.Revision})
}

func (a *App) retention(ctx context.Context) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		cutoff := time.Now().UTC().AddDate(0, 0, -60)
		for _, item := range []any{&Log{}, &Audit{}, &Notification{}, &SessionDevice{}} {
			if err := a.DB.WithContext(ctx).Where("created_at < ?", cutoff).Delete(item).Error; err != nil && ctx.Err() == nil {
				log.Print("retention failed")
			}
		}
		_ = a.DB.WithContext(ctx).Where("day < ?", cutoff.Format("2006-01-02")).Delete(&DailyUsage{}).Error
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func success(c *gin.Context, data any) { c.JSON(200, gin.H{"success": true, "data": data}) }
func failure(c *gin.Context, status int, code, message string) {
	c.Set("errorCode", code)
	message = translateText(requestLanguage(c), message)
	c.Header("Content-Language", requestLanguage(c))
	c.AbortWithStatusJSON(status, gin.H{"success": false, "error": gin.H{"code": code, "message": message}, "request_id": c.GetString("requestID")})
}
func randomKey(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("random source unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func digest(s string) string          { b := sha256.Sum256([]byte(s)); return hex.EncodeToString(b[:]) }
func currentUser(c *gin.Context) User { return c.MustGet("user").(User) }
