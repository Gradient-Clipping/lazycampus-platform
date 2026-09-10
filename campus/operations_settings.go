package campus

import (
	"errors"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/Gradient-Clipping/lazycampus-platform/common"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func readSetting[T any](db *gorm.DB, name string, fallback T) (T, error) {
	var row Setting
	err := db.Where("name = ?", name).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fallback, nil
	}
	if err != nil {
		return fallback, err
	}
	err = common.Unmarshal([]byte(row.Value), &fallback)
	return fallback, err
}
func (a *App) policy() (PlatformPolicy, error) {
	return readSetting(a.DB, "operations.policy.v1", defaultPolicy())
}
func (a *App) site() (SiteContent, error) {
	return readSetting(a.DB, "operations.site.v1", defaultSite())
}

func migrateOperations(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		for name, value := range map[string]any{"operations.policy.v1": defaultPolicy(), "operations.site.v1": defaultSite()} {
			raw, err := common.Marshal(value)
			if err != nil {
				return err
			}
			if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Setting{Name: name, Value: string(raw)}).Error; err != nil {
				return err
			}
		}
		for _, endpoint := range Catalog {
			row := EndpointPolicy{Path: endpoint.Path, Name: endpoint.Name, Scope: endpoint.Scope, Description: endpoint.Description, Status: "operational", RateLimit: 20, DailyQuota: DefaultDailyQuota, Cooldown: endpoint.Cooldown}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
		}
		if err := migrateLogoWebP(tx); err != nil {
			return err
		}
		if err := migrateDailyQuota1000(tx); err != nil {
			return err
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Setting{Name: "migration.operations.v1", Value: "complete"})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		if err := tx.Model(&User{}).Where("daily_quota = ? AND rate_limit = ?", DefaultDailyQuota, DefaultRateLimit).Update("inherit_limits", true).Error; err != nil {
			return err
		}
		if err := tx.Model(&Log{}).Where("id > 0").Update("admitted", true).Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE platform_logs l JOIN platform_users u ON u.id=l.user_id SET l.user_name=u.username WHERE l.user_name IS NULL OR l.user_name='' ").Error; err != nil {
			return err
		}
		return tx.Exec("UPDATE platform_logs l JOIN platform_tokens t ON t.id=l.token_id SET l.token_name=t.name WHERE l.token_name IS NULL OR l.token_name='' ").Error
	})
}

func migrateLogoWebP(tx *gorm.DB) error {
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Setting{Name: "migration.logo-webp.v1", Value: "complete"})
	if result.Error != nil || result.RowsAffected == 0 {
		return result.Error
	}
	var row Setting
	if err := lockForUpdate(tx).Where("name = ?", "operations.site.v1").First(&row).Error; err != nil {
		return err
	}
	var value map[string]any
	if err := common.Unmarshal([]byte(row.Value), &value); err != nil {
		return err
	}
	if value["logo_url"] != "/logo.png" {
		return nil
	}
	value["logo_url"] = "/logo.webp"
	raw, err := common.Marshal(value)
	if err != nil {
		return err
	}
	return tx.Model(&row).Update("value", string(raw)).Error
}

type settingsInput[T any] struct {
	Value     T         `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

var errSettingsConflict = errors.New("settings changed")

func saveSetting[T any](a *App, c *gin.Context, name string, input settingsInput[T], apply func(*gorm.DB) error) {
	err := a.DB.Transaction(func(tx *gorm.DB) error {
		var row Setting
		if err := lockForUpdate(tx).Where("name = ?", name).First(&row).Error; err != nil {
			return err
		}
		if !row.UpdatedAt.Equal(input.UpdatedAt) {
			return errSettingsConflict
		}
		value, err := common.Marshal(input.Value)
		if err != nil {
			return err
		}
		before := row.Value
		if err = tx.Model(&row).Update("value", string(value)).Error; err != nil {
			return err
		}
		if apply != nil {
			if err = apply(tx); err != nil {
				return err
			}
		}
		return tx.Create(&Audit{ActorID: currentUser(c).ID, Action: "settings.update", Target: name, Details: auditDetails(gin.H{"before": before, "after": string(value)})}).Error
	})
	if errors.Is(err, errSettingsConflict) {
		failure(c, 409, "SETTINGS_CONFLICT", "设置已被其他管理员修改，请刷新后重试")
		return
	}
	if err != nil {
		failure(c, 503, "UNAVAILABLE", "无法保存设置")
		return
	}
	success(c, gin.H{"updated": true})
}
func (a *App) getPolicy(c *gin.Context) { a.settingResponse(c, "operations.policy.v1") }
func (a *App) getSite(c *gin.Context)   { a.settingResponse(c, "operations.site.v1") }
func (a *App) settingResponse(c *gin.Context, name string) {
	var row Setting
	if a.DB.Where("name = ?", name).First(&row).Error != nil {
		failure(c, 503, "UNAVAILABLE", "无法读取设置")
		return
	}
	var value any
	if common.Unmarshal([]byte(row.Value), &value) != nil {
		failure(c, 503, "UNAVAILABLE", "设置不可用")
		return
	}
	success(c, gin.H{"value": value, "updated_at": row.UpdatedAt})
}
func (a *App) updatePolicy(c *gin.Context) {
	var input settingsInput[PlatformPolicy]
	if c.ShouldBindJSON(&input) != nil {
		failure(c, 400, "INVALID_INPUT", "请检查额度设置")
		return
	}
	p := input.Value
	if p.DailyQuota < 1 || p.DailyQuota > 50000 || p.RateLimit < 1 || p.RateLimit > 300 || p.AppQuota < 1 || p.AppQuota > 10000000 || p.AppDays < 1 || p.AppDays > 365 || p.MaxApps < 1 || p.MaxApps > 50 || p.Burst < 1 || p.Burst > 30 || p.UserConcurrency < 1 || p.UserConcurrency > 10 || p.GlobalConcurrency < 1 || p.GlobalConcurrency > 100 || p.UserConcurrency > p.GlobalConcurrency {
		failure(c, 400, "INVALID_INPUT", "额度或限流设置超出允许范围")
		return
	}
	saveSetting(a, c, "operations.policy.v1", input, func(tx *gorm.DB) error {
		return tx.Model(&User{}).Where("inherit_limits = ?", true).Updates(map[string]any{"daily_quota": p.DailyQuota, "rate_limit": p.RateLimit}).Error
	})
}
func (a *App) updateSite(c *gin.Context) {
	var input settingsInput[SiteContent]
	if c.ShouldBindJSON(&input) != nil {
		failure(c, 400, "INVALID_INPUT", "请检查站点设置")
		return
	}
	s := &input.Value
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" || len([]rune(s.Name)) > 60 || len([]rune(s.Description)) > 300 || len(s.ServiceMessage) > 1000 || len(s.FAQ) > 30 || !validServiceStatus(s.ServiceStatus) || !safePublicURL(s.LogoURL, true) || !safePublicURL(s.HelpURL, false) || !validEmail(s.SupportEmail) {
		failure(c, 400, "INVALID_INPUT", "站点设置无效")
		return
	}
	seen := map[string]bool{}
	for i := range s.FAQ {
		faq := &s.FAQ[i]
		if faq.ID == "" {
			faq.ID = randomKey(18)
		}
		if !opaqueID.MatchString(faq.ID) || seen[faq.ID] {
			failure(c, 400, "INVALID_INPUT", "FAQ 标识无效或重复")
			return
		}
		seen[faq.ID] = true
		if strings.TrimSpace(faq.Question) == "" || len([]rune(faq.Question)) > 200 || len([]rune(faq.Answer)) > 3000 {
			failure(c, 400, "INVALID_INPUT", "请检查 FAQ 内容")
			return
		}
	}
	saveSetting(a, c, "operations.site.v1", input, nil)
}
func validServiceStatus(s string) bool {
	return s == "operational" || s == "maintenance" || s == "degraded"
}
func validEmail(s string) bool {
	if s == "" {
		return true
	}
	address, err := mail.ParseAddress(s)
	return err == nil && address.Address == s && len(s) <= 254 && !strings.ContainsAny(s, "\r\n")
}
func safePublicURL(s string, localOnly bool) bool {
	if s == "" {
		return !localOnly
	}
	if strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "//") && !strings.ContainsAny(s, "\\\r\n") {
		return true
	}
	if localOnly {
		return false
	}
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil
}

type catalogEntry struct {
	Endpoint
	Documentation *endpointReference `json:"documentation,omitempty"`
	Status        string             `json:"status"`
	Message       string             `json:"message"`
	RateLimit     int64              `json:"rate_limit"`
	DailyQuota    int64              `json:"daily_quota"`
	UpdatedAt     time.Time          `json:"updated_at"`
}

func (a *App) catalogEntries() ([]catalogEntry, error) {
	var policies []EndpointPolicy
	if err := a.DB.Find(&policies).Error; err != nil {
		return nil, err
	}
	byPath := map[string]EndpointPolicy{}
	for _, p := range policies {
		byPath[p.Path] = p
	}
	entries := make([]catalogEntry, 0, len(Catalog))
	for _, e := range Catalog {
		p, ok := byPath[e.Path]
		if !ok {
			return nil, errors.New("endpoint policy missing")
		}
		e.Name = p.Name
		e.Scope = p.Scope
		e.Description = p.Description
		e.Cooldown = p.Cooldown
		entries = append(entries, catalogEntry{Endpoint: e, Status: p.Status, Message: p.Message, RateLimit: p.RateLimit, DailyQuota: p.DailyQuota, UpdatedAt: p.UpdatedAt})
	}
	return entries, nil
}
func (a *App) catalog(c *gin.Context) {
	entries, err := a.catalogEntries()
	if err != nil {
		failure(c, 503, "UNAVAILABLE", "无法读取接口目录")
		return
	}
	for i := range entries {
		doc := reference.Endpoints[entries[i].Path]
		doc.ResponseSchema = expandReference(doc.ResponseSchema).(map[string]any)
		entries[i].Documentation = &doc
	}
	success(c, entries)
}
func (a *App) updateEndpoint(c *gin.Context) {
	var input EndpointPolicy
	if c.ShouldBindJSON(&input) != nil {
		failure(c, 400, "INVALID_INPUT", "接口设置无效")
		return
	}
	base, ok := endpointFor(input.Path)
	if !ok || !validScope(input.Scope) || strings.TrimSpace(input.Name) == "" || len([]rune(input.Name)) > 80 || len([]rune(input.Description)) > 4000 || len(input.Message) > 500 || input.Cooldown < base.Cooldown || input.Cooldown > 3600 || input.RateLimit < 1 || input.RateLimit > 300 || input.DailyQuota < 1 || input.DailyQuota > 50000 || (input.Status != "disabled" && !validServiceStatus(input.Status)) {
		failure(c, 400, "INVALID_INPUT", "接口设置超出允许范围")
		return
	}
	err := a.DB.Transaction(func(tx *gorm.DB) error {
		var old EndpointPolicy
		if err := lockForUpdate(tx).First(&old, "path = ?", input.Path).Error; err != nil {
			return err
		}
		if !old.UpdatedAt.Equal(input.UpdatedAt) {
			return errSettingsConflict
		}
		before := old
		if err := tx.Model(&old).Updates(map[string]any{"name": input.Name, "scope": input.Scope, "description": input.Description, "status": input.Status, "message": input.Message, "rate_limit": input.RateLimit, "daily_quota": input.DailyQuota, "cooldown": input.Cooldown}).Error; err != nil {
			return err
		}
		return tx.Create(&Audit{ActorID: currentUser(c).ID, Action: "endpoint.update", Target: input.Path + " " + input.Status, Details: auditDetails(gin.H{"before": before, "after": input})}).Error
	})
	if errors.Is(err, errSettingsConflict) {
		failure(c, 409, "SETTINGS_CONFLICT", "接口设置已变化，请刷新")
		return
	}
	if err != nil {
		failure(c, 503, "UNAVAILABLE", "无法保存接口")
		return
	}
	success(c, gin.H{"updated": true})
}
func (a *App) publicSite(c *gin.Context) {
	s, err := a.site()
	if err != nil {
		failure(c, 503, "UNAVAILABLE", "站点配置暂不可用")
		return
	}
	p, err := a.policy()
	if err != nil {
		failure(c, 503, "UNAVAILABLE", "额度配置暂不可用")
		return
	}
	endpoints, err := a.catalogEntries()
	if err != nil {
		failure(c, 503, "UNAVAILABLE", "接口状态暂不可用")
		return
	}
	statuses := []gin.H{}
	for _, e := range endpoints {
		statuses = append(statuses, gin.H{"path": e.Path, "name": e.Name, "status": e.Status, "message": e.Message})
	}
	success(c, gin.H{"site": s, "limits": p, "endpoints": statuses, "email_available": a.Config.SenderAPIKey != "" && a.Config.SenderFrom != ""})
}
