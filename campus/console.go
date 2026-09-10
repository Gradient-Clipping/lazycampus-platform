package campus

import (
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type tokenInput struct {
	Name          string   `json:"name"`
	Scopes        []string `json:"scopes"`
	AllowedIPs    string   `json:"allowed_ips"`
	DailyQuota    int64    `json:"daily_quota"`
	RateLimit     int64    `json:"rate_limit"`
	Quota         int64    `json:"quota"`
	ExpiresInDays int      `json:"expires_in_days"`
	NeverExpires  bool     `json:"never_expires"`
	Enabled       *bool    `json:"enabled"`
}

func (input *tokenInput) validate(u User) error {
	input.Name = strings.TrimSpace(input.Name)
	if len([]rune(input.Name)) < 1 || len([]rune(input.Name)) > 60 {
		return errors.New("应用名称需为 1–60 个字符")
	}
	if len(input.Scopes) < 1 || len(input.Scopes) > 10 {
		return errors.New("请选择接口权限")
	}
	for _, scope := range input.Scopes {
		if !validScope(scope) {
			return errors.New("接口权限无效")
		}
	}
	slices.Sort(input.Scopes)
	input.Scopes = slices.Compact(input.Scopes)
	if input.DailyQuota < 1 || input.DailyQuota > u.DailyQuota || input.RateLimit < 1 || input.RateLimit > u.RateLimit || input.Quota < 0 || input.Quota > 10_000_000 {
		return errors.New("应用额度不能超过账户限制")
	}
	if !input.NeverExpires && (input.ExpiresInDays < 1 || input.ExpiresInDays > 365) {
		return errors.New("有效期需为 1–365 天")
	}
	if len(input.AllowedIPs) > 1000 || len(strings.Fields(input.AllowedIPs)) > 20 {
		return errors.New("IP 白名单最多 20 项")
	}
	for _, ip := range strings.Fields(input.AllowedIPs) {
		_, addressErr := netip.ParseAddr(ip)
		_, prefixErr := netip.ParsePrefix(ip)
		if addressErr != nil && prefixErr != nil {
			return errors.New("IP 地址或网段无效")
		}
	}
	input.AllowedIPs = strings.Join(strings.Fields(input.AllowedIPs), " ")
	return nil
}

func (input tokenInput) expiration() *time.Time {
	if input.NeverExpires {
		return nil
	}
	expires := time.Now().UTC().Add(time.Duration(input.ExpiresInDays) * 24 * time.Hour)
	return &expires
}

// A document visit is separate from background session refreshes and API usage.
func (a *App) visit(c *gin.Context) {
	now := time.Now().UTC()
	user := currentUser(c)
	if err := a.DB.Model(&user).Update("last_login_at", gorm.Expr("GREATEST(last_login_at, ?)", now)).Error; err != nil {
		failure(c, 503, "UNAVAILABLE", "无法更新登录时间")
		return
	}
	success(c, gin.H{"last_login_at": now})
}

func (a *App) me(c *gin.Context) {
	p, err := a.policy()
	if err != nil {
		failure(c, 503, "UNAVAILABLE", "无法读取账户设置")
		return
	}
	success(c, gin.H{"user": currentUser(c), "csrf_token": c.MustGet("session").(browserSession).CSRF, "limits": p})
}

func (a *App) listTokens(c *gin.Context) {
	tokens := []Token{}
	u := currentUser(c)
	if err := a.DB.Where("user_id = ?", u.ID).Order("id DESC").Find(&tokens).Error; err != nil {
		failure(c, 503, "UNAVAILABLE", "无法读取应用")
		return
	}
	usage := map[uint64]int64{}
	for _, token := range tokens {
		var d DailyUsage
		result := a.DB.Where("bucket = ? AND day = ?", fmt.Sprintf("token:%d", token.ID), time.Now().UTC().Format("2006-01-02")).First(&d)
		if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
			failure(c, 503, "UNAVAILABLE", "无法读取额度")
			return
		}
		usage[token.ID] = d.Requests
	}
	success(c, gin.H{"items": tokens, "daily_usage": usage})
}

func (a *App) createToken(c *gin.Context) {
	u := currentUser(c)
	p, err := a.policy()
	if err != nil {
		failure(c, 503, "UNAVAILABLE", "无法读取应用设置")
		return
	}
	var input tokenInput
	if c.ShouldBindJSON(&input) != nil {
		failure(c, 400, "INVALID_INPUT", "请检查应用设置")
		return
	}
	if err := input.validate(u); err != nil {
		failure(c, 400, "INVALID_INPUT", err.Error())
		return
	}
	key := "lc_" + randomKey(32)
	token := Token{UserID: u.ID, Name: input.Name, KeyHash: digest(key), KeyPrefix: key[:12], Scopes: strings.Join(input.Scopes, " "), AllowedIPs: input.AllowedIPs, Enabled: true, DailyQuota: input.DailyQuota, RateLimit: input.RateLimit, Quota: input.Quota, ExpiresAt: input.expiration()}
	err = a.DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&u, u.ID).Error; err != nil {
			return err
		}
		if !u.Enabled {
			return errDisabled
		}
		var count int64
		if err := tx.Model(&Token{}).Where("user_id = ?", u.ID).Count(&count).Error; err != nil {
			return err
		}
		if count >= int64(p.MaxApps) {
			return errors.New("application limit")
		}
		if err := tx.Create(&token).Error; err != nil {
			return err
		}
		return tx.Create(&Audit{ActorID: u.ID, Action: "token.create", Target: strconv.FormatUint(token.ID, 10)}).Error
	})
	if err != nil {
		failure(c, 400, "APPLICATION_LIMIT", fmt.Sprintf("每个账户最多保留 %d 个应用，请先删除不用的应用", p.MaxApps))
		return
	}
	success(c, gin.H{"token": token, "key": key})
}

func (a *App) updateToken(c *gin.Context) {
	u := currentUser(c)
	var input tokenInput
	if c.ShouldBindJSON(&input) != nil {
		failure(c, 400, "INVALID_INPUT", "请检查应用设置")
		return
	}
	if err := input.validate(u); err != nil {
		failure(c, 400, "INVALID_INPUT", err.Error())
		return
	}
	err := a.DB.Transaction(func(tx *gorm.DB) error {
		// Use the same lock order as quota admission and token rotation.
		if err := lockForUpdate(tx).First(&u, u.ID).Error; err != nil {
			return err
		}
		var token Token
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", c.Param("id"), u.ID).First(&token).Error; err != nil {
			return err
		}
		if token.RevokedAt != nil || (input.Quota > 0 && input.Quota < token.UsedQuota) {
			return errors.New("quota below usage")
		}
		enabled := token.Enabled
		if input.Enabled != nil {
			enabled = *input.Enabled
		}
		if err := tx.Model(&token).Updates(map[string]any{"name": input.Name, "scopes": strings.Join(input.Scopes, " "), "allowed_ips": input.AllowedIPs, "daily_quota": input.DailyQuota, "rate_limit": input.RateLimit, "quota": input.Quota, "enabled": enabled, "expires_at": input.expiration()}).Error; err != nil {
			return err
		}
		return tx.Create(&Audit{ActorID: u.ID, Action: "token.update", Target: c.Param("id")}).Error
	})
	if err != nil {
		failure(c, 400, "UPDATE_REJECTED", "应用不存在或总额度低于已用次数")
		return
	}
	success(c, gin.H{"updated": true})
}

func (a *App) rotateToken(c *gin.Context) {
	u := currentUser(c)
	key := "lc_" + randomKey(32)
	err := a.DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&u, u.ID).Error; err != nil {
			return err
		}
		result := tx.Model(&Token{}).Where("id = ? AND user_id = ? AND revoked_at IS NULL AND suspended = ?", c.Param("id"), u.ID, false).Updates(map[string]any{"key_hash": digest(key), "key_prefix": key[:12]})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return tx.Create(&Audit{ActorID: u.ID, Action: "token.rotate", Target: c.Param("id")}).Error
	})
	if err != nil {
		failure(c, 404, "NOT_FOUND", "应用不存在")
		return
	}
	success(c, gin.H{"key": key})
}

func (a *App) deleteToken(c *gin.Context) {
	u := currentUser(c)
	err := a.DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&u, u.ID).Error; err != nil {
			return err
		}
		result := tx.Where("id = ? AND user_id = ?", c.Param("id"), u.ID).Delete(&Token{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return tx.Create(&Audit{ActorID: u.ID, Action: "token.delete", Target: c.Param("id")}).Error
	})
	if err != nil {
		failure(c, 404, "NOT_FOUND", "应用不存在")
		return
	}
	success(c, gin.H{"deleted": true})
}

func (a *App) dashboard(c *gin.Context) {
	u := currentUser(c)
	var total, apps, successful, failed int64
	start := time.Now().UTC().Add(-24 * time.Hour)
	for _, query := range []struct {
		db     *gorm.DB
		target *int64
	}{
		{a.DB.Model(&Log{}).Where("user_id = ?", u.ID), &total},
		{a.DB.Model(&Token{}).Where("user_id = ? AND enabled = ? AND suspended = ? AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)", u.ID, true, false, time.Now().UTC()), &apps},
		{a.DB.Model(&Log{}).Where("user_id = ? AND created_at >= ? AND status >= ? AND status < ?", u.ID, start, 200, 300), &successful},
		{a.DB.Model(&Log{}).Where("user_id = ? AND created_at >= ? AND status >= ?", u.ID, start, 400), &failed},
	} {
		if query.db.Count(query.target).Error != nil {
			failure(c, 503, "UNAVAILABLE", "无法读取统计")
			return
		}
	}
	series := []DailyUsage{}
	if a.DB.Where("bucket = ? AND day >= ?", fmt.Sprintf("user:%d", u.ID), time.Now().UTC().AddDate(0, 0, -13).Format("2006-01-02")).Order("day").Find(&series).Error != nil {
		failure(c, 503, "UNAVAILABLE", "无法读取统计")
		return
	}
	today := int64(0)
	for _, d := range series {
		if d.Day == time.Now().UTC().Format("2006-01-02") {
			today = d.Requests
		}
	}
	success(c, gin.H{"total_requests": total, "active_apps": apps, "successful_24h": successful, "failed_24h": failed, "today_requests": today, "daily_quota": u.DailyQuota, "rate_limit": u.RateLimit, "series": series, "reset_at": time.Now().UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)})
}

func pagination(c *gin.Context) (int, int) {
	p, _ := strconv.Atoi(c.Query("page"))
	size, _ := strconv.Atoi(c.Query("page_size"))
	if p < 1 {
		p = 1
	}
	if size < 1 {
		size = 25
	}
	return min(p, 10000), min(size, 100)
}
func (a *App) listUsers(c *gin.Context) {
	query := a.DB.Model(&User{})
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		if len(search) > 100 {
			failure(c, 400, "INVALID_INPUT", "搜索内容过长")
			return
		}
		query = query.Where("username LIKE ? OR display_name LIKE ?", "%"+search+"%", "%"+search+"%")
	}
	var total int64
	users := []User{}
	page, size := pagination(c)
	if query.Count(&total).Error != nil || query.Order("id DESC").Offset((page-1)*size).Limit(size).Find(&users).Error != nil {
		failure(c, 503, "UNAVAILABLE", "无法读取用户")
		return
	}
	success(c, gin.H{"items": users, "total": total, "page": page, "page_size": size})
}

func (a *App) updateUser(c *gin.Context) {
	id, parseErr := strconv.ParseUint(c.Param("id"), 10, 64)
	if parseErr != nil || id == 0 {
		failure(c, 400, "INVALID_INPUT", "用户标识无效")
		return
	}
	var input struct {
		Enabled       bool  `json:"enabled"`
		InheritLimits bool  `json:"inherit_limits"`
		DailyQuota    int64 `json:"daily_quota"`
		RateLimit     int64 `json:"rate_limit"`
	}
	if c.ShouldBindJSON(&input) != nil || input.DailyQuota < 1 || input.DailyQuota > 50000 || input.RateLimit < 1 || input.RateLimit > 300 {
		failure(c, 400, "INVALID_INPUT", "每日额度需为 1–50,000 次，每分钟限制需为 1–300 次")
		return
	}
	actor := currentUser(c)
	if input.InheritLimits {
		p, err := a.policy()
		if err != nil {
			failure(c, 503, "UNAVAILABLE", "无法读取默认额度")
			return
		}
		input.DailyQuota = p.DailyQuota
		input.RateLimit = p.RateLimit
	}
	err := a.DB.Transaction(func(tx *gorm.DB) error {
		var target User
		if err := lockForUpdate(tx).First(&target, id).Error; err != nil {
			return err
		}
		if target.ID == actor.ID && !input.Enabled {
			return errors.New("cannot disable self")
		}
		if err := tx.Model(&target).Updates(map[string]any{"enabled": input.Enabled, "daily_quota": input.DailyQuota, "rate_limit": input.RateLimit, "inherit_limits": input.InheritLimits}).Error; err != nil {
			return err
		}
		return tx.Create(&Audit{ActorID: actor.ID, Action: "user.update", Target: fmt.Sprintf("%d enabled=%t daily=%d rpm=%d", target.ID, input.Enabled, input.DailyQuota, input.RateLimit)}).Error
	})
	if err != nil {
		failure(c, 400, "UPDATE_REJECTED", "用户不存在或操作不允许")
		return
	}
	success(c, gin.H{"updated": true})
}

func (a *App) notice(c *gin.Context) {
	var item Announcement
	err := a.DB.Order("id DESC").First(&item).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		failure(c, 503, "UNAVAILABLE", "无法读取公告")
		return
	}
	success(c, gin.H{"content": item.Content, "updated_at": item.CreatedAt})
}
func (a *App) announcements(c *gin.Context) {
	items := []Announcement{}
	if err := a.DB.Order("id DESC").Limit(50).Find(&items).Error; err != nil {
		failure(c, 503, "UNAVAILABLE", "无法读取公告")
		return
	}
	success(c, gin.H{"items": items})
}
func (a *App) updateNotice(c *gin.Context) {
	var input struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if c.ShouldBindJSON(&input) != nil || len([]rune(input.Content)) > 8000 || len([]rune(input.Title)) > 120 || strings.TrimSpace(input.Content) == "" {
		failure(c, 400, "INVALID_INPUT", "请填写有效的公告标题和内容")
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		input.Title = "系统公告"
	}
	item := Announcement{Title: input.Title, Content: strings.TrimSpace(input.Content)}
	err := a.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		return tx.Create(&Audit{ActorID: currentUser(c).ID, Action: "notice.publish", Target: strconv.FormatUint(item.ID, 10)}).Error
	})
	if err != nil {
		failure(c, 503, "UNAVAILABLE", "无法保存公告")
		return
	}
	success(c, gin.H{"updated": true})
}
func (a *App) deleteAnnouncement(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		failure(c, 400, "INVALID_INPUT", "公告编号无效")
		return
	}
	err = a.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Delete(&Announcement{}, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return tx.Create(&Audit{ActorID: currentUser(c).ID, Action: "notice.withdraw", Target: strconv.FormatUint(id, 10)}).Error
	})
	if err != nil {
		failure(c, 400, "UPDATE_REJECTED", "无法撤回公告")
		return
	}
	success(c, gin.H{"deleted": true})
}
func (a *App) audits(c *gin.Context) {
	var total int64
	entries := []Audit{}
	page, size := pagination(c)
	query := a.DB.Model(&Audit{})
	if query.Count(&total).Error != nil || query.Order("id DESC").Offset((page-1)*size).Limit(size).Find(&entries).Error != nil {
		failure(c, 503, "UNAVAILABLE", "无法读取审计")
		return
	}
	success(c, gin.H{"items": entries, "total": total, "page": page, "page_size": size})
}
func (a *App) system(c *gin.Context) {
	pool, err := a.DB.DB()
	dbOK := err == nil && pool.PingContext(c.Request.Context()) == nil
	redisOK := a.Redis.Ping(c.Request.Context()).Err() == nil
	var users, tokens, requests int64
	for _, item := range []struct {
		model any
		count *int64
	}{{&User{}, &users}, {&Token{}, &tokens}, {&Log{}, &requests}} {
		if a.DB.Model(item.model).Count(item.count).Error != nil {
			failure(c, 503, "UNAVAILABLE", "无法读取系统状态")
			return
		}
	}
	p, err := a.policy()
	if err != nil {
		failure(c, 503, "UNAVAILABLE", "无法读取限制设置")
		return
	}
	success(c, gin.H{"revision": a.Config.Revision, "database": dbOK, "redis": redisOK, "users": users, "tokens": tokens, "requests": requests, "default_daily_quota": p.DailyQuota, "default_rate_limit": p.RateLimit, "concurrency_per_user": p.UserConcurrency, "concurrency_global": p.GlobalConcurrency, "log_retention_days": 60, "email_available": a.emailAvailable()})
}
