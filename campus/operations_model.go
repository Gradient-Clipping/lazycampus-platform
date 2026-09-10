package campus

import "time"

// Runtime policy is versioned and audited independently of deployment configuration.
type PlatformPolicy struct {
	DailyQuota        int64 `json:"daily_quota"`
	RateLimit         int64 `json:"rate_limit"`
	AppQuota          int64 `json:"app_quota"`
	AppDays           int   `json:"app_days"`
	MaxApps           int   `json:"max_apps"`
	Burst             int   `json:"burst"`
	UserConcurrency   int   `json:"user_concurrency"`
	GlobalConcurrency int   `json:"global_concurrency"`
}

func defaultPolicy() PlatformPolicy {
	return PlatformPolicy{DailyQuota: DefaultDailyQuota, RateLimit: 20, AppQuota: 100000, AppDays: 90, MaxApps: 10, Burst: 10, UserConcurrency: 3, GlobalConcurrency: 20}
}

type EndpointPolicy struct {
	Path        string    `json:"path" gorm:"primaryKey;size:160"`
	Name        string    `json:"name" gorm:"size:120"`
	Scope       string    `json:"scope" gorm:"size:64"`
	Description string    `json:"description" gorm:"type:text"`
	Status      string    `json:"status" gorm:"size:20"`
	Message     string    `json:"message" gorm:"size:500"`
	RateLimit   int64     `json:"rate_limit"`
	DailyQuota  int64     `json:"daily_quota"`
	Cooldown    int       `json:"cooldown_seconds"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (EndpointPolicy) TableName() string { return "platform_endpoints" }

type FAQ struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
}
type SiteContent struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	LogoURL        string `json:"logo_url"`
	HelpURL        string `json:"help_url"`
	SupportEmail   string `json:"support_email"`
	ServiceStatus  string `json:"service_status"`
	ServiceMessage string `json:"service_message"`
	FAQ            []FAQ  `json:"faq"`
}

func defaultSite() SiteContent {
	return SiteContent{Name: "Lazy Campus", Description: "免费校园开放平台", LogoURL: "/logo.webp", HelpURL: "/api-reference", ServiceStatus: "operational", FAQ: []FAQ{
		{ID: "getting-started-campus", Question: "如何开始调用？", Answer: "使用学校身份进入控制台，创建应用并选择接口权限，将应用密钥保存到自己的服务或脚本中。接口文档提供调用示例与在线调试。"},
		{ID: "daily-quota-campus-api", Question: "调用额度如何计算？", Answer: "所有应用共用账户每日额度，同时受应用和接口各自的限制。每日额度在 UTC 00:00（北京时间 08:00）重置；已接纳的调用计入额度，权限或限流拒绝不扣额度。"},
		{ID: "campus-data-permissions", Question: "应用可以访问哪些数据？", Answer: "应用只能读取密钥所属学校用户的数据，且仅能调用授权范围内的接口。开放 API 不支持修改学校数据。"},
	}}
}

type NotificationPreference struct {
	Language      string    `json:"language" gorm:"size:5"`
	UserID        uint64    `json:"user_id" gorm:"primaryKey"`
	Email         string    `json:"email" gorm:"size:254"`
	VerifiedEmail string    `json:"-" gorm:"size:254"`
	InApp         bool      `json:"in_app"`
	EmailEnabled  bool      `json:"email_enabled"`
	QuotaEnabled  bool      `json:"quota_enabled"`
	QuotaPercent  int       `json:"quota_percent"`
	ErrorsEnabled bool      `json:"errors_enabled"`
	ErrorPercent  int       `json:"error_percent"`
	ErrorMinimum  int       `json:"error_minimum"`
	RateEnabled   bool      `json:"rate_enabled"`
	ExpiryEnabled bool      `json:"expiry_enabled"`
	ExpiryDays    int       `json:"expiry_days"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (NotificationPreference) TableName() string { return "platform_notification_preferences" }
func defaultNotificationPreference(id uint64) NotificationPreference {
	return NotificationPreference{UserID: id, InApp: true, QuotaEnabled: true, QuotaPercent: 80, ErrorsEnabled: true, ErrorPercent: 30, ErrorMinimum: 10, RateEnabled: true, ExpiryEnabled: true, ExpiryDays: 7}
}

type Notification struct {
	ID            uint64     `json:"id" gorm:"primaryKey"`
	UserID        uint64     `json:"user_id" gorm:"index:notification_user_time,priority:1"`
	Dedupe        string     `json:"-" gorm:"size:160;uniqueIndex"`
	Kind          string     `json:"kind" gorm:"size:40"`
	Title         string     `json:"title" gorm:"size:160"`
	Content       string     `json:"content" gorm:"type:text"`
	ReadAt        *time.Time `json:"read_at"`
	InApp         bool       `json:"in_app"`
	EmailStatus   string     `json:"email_status" gorm:"size:20"`
	EmailAttempts int        `json:"-"`
	NextAttemptAt time.Time  `json:"-"`
	CreatedAt     time.Time  `json:"created_at" gorm:"index:notification_user_time,priority:2"`
}

func (Notification) TableName() string { return "platform_notifications" }

type SessionDevice struct {
	ID           string     `json:"id" gorm:"primaryKey;size:64"`
	UserID       uint64     `json:"user_id" gorm:"index"`
	Device       string     `json:"device" gorm:"size:160"`
	IP           string     `json:"ip" gorm:"size:64"`
	CreatedAt    time.Time  `json:"created_at"`
	LastActiveAt time.Time  `json:"last_active_at"`
	ExpiresAt    time.Time  `json:"expires_at"`
	RevokedAt    *time.Time `json:"revoked_at"`
}

func (SessionDevice) TableName() string { return "platform_session_devices" }
