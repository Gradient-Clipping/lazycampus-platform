// Campus request quotas replace new-api's monetary and model-token accounting.
package campus

import (
	"errors"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

const DefaultDailyQuota = 1000
const DefaultRateLimit = 20

type User struct {
	ID            uint64    `json:"id" gorm:"primaryKey"`
	Subject       string    `json:"-" gorm:"size:128;uniqueIndex;not null"`
	IdentityID    string    `json:"-" gorm:"size:64;index"`
	Username      string    `json:"username" gorm:"size:128;index"`
	DisplayName   string    `json:"display_name" gorm:"size:160"`
	Role          string    `json:"role" gorm:"size:16"`
	Enabled       bool      `json:"enabled"`
	DailyQuota    int64     `json:"daily_quota"`
	RateLimit     int64     `json:"rate_limit"`
	InheritLimits bool      `json:"inherit_limits"`
	CreatedAt     time.Time `json:"created_at"`
	LastLoginAt   time.Time `json:"last_login_at" gorm:"autoCreateTime"`
}

func (User) TableName() string { return "platform_users" }

type Token struct {
	ID          uint64     `json:"id" gorm:"primaryKey"`
	UserID      uint64     `json:"user_id" gorm:"index;not null"`
	Name        string     `json:"name" gorm:"size:80"`
	KeyHash     string     `json:"-" gorm:"size:64;uniqueIndex;not null"`
	KeyPrefix   string     `json:"key_prefix" gorm:"size:20"`
	Scopes      string     `json:"scopes" gorm:"size:512"`
	AllowedIPs  string     `json:"allowed_ips" gorm:"size:1024"`
	Enabled     bool       `json:"enabled"`
	Suspended   bool       `json:"suspended"`
	RevokedAt   *time.Time `json:"revoked_at"`
	AdminReason string     `json:"admin_reason" gorm:"size:500"`
	DailyQuota  int64      `json:"daily_quota"`
	RateLimit   int64      `json:"rate_limit"`
	Quota       int64      `json:"quota"`
	UsedQuota   int64      `json:"used_quota"`
	ExpiresAt   *time.Time `json:"expires_at"`
	CreatedAt   time.Time  `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
}

func (Token) TableName() string { return "platform_tokens" }

type DailyUsage struct {
	Bucket   string `json:"bucket" gorm:"primaryKey;size:64"`
	Day      string `json:"day" gorm:"primaryKey;size:10"`
	Requests int64  `json:"requests"`
}

func (DailyUsage) TableName() string { return "platform_daily_usage" }

type Log struct {
	ID                uint64    `json:"id" gorm:"primaryKey"`
	UserID            uint64    `json:"user_id" gorm:"index:log_user_time,priority:1"`
	TokenID           uint64    `json:"token_id" gorm:"index"`
	RequestID         string    `json:"request_id" gorm:"size:64;index"`
	Endpoint          string    `json:"endpoint" gorm:"size:160"`
	Method            string    `json:"method" gorm:"size:8"`
	Status            int       `json:"status"`
	Admitted          bool      `json:"admitted"`
	UserName          string    `json:"user_name" gorm:"size:128"`
	TokenName         string    `json:"token_name" gorm:"size:80"`
	ErrorCode         string    `json:"error_code" gorm:"size:80;index"`
	ErrorMessage      string    `json:"error_message" gorm:"size:500"`
	UpstreamRequestID string    `json:"upstream_request_id" gorm:"size:128;index"`
	ClientIP          string    `json:"client_ip" gorm:"size:64"`
	RetryAfter        int64     `json:"retry_after"`
	LimitScope        string    `json:"limit_scope" gorm:"size:32"`
	DurationMS        int64     `json:"duration_ms"`
	CreatedAt         time.Time `json:"created_at" gorm:"index:log_user_time,priority:2;index"`
}

func (Log) TableName() string { return "platform_logs" }

type Audit struct {
	ID        uint64    `json:"id" gorm:"primaryKey"`
	ActorID   uint64    `json:"actor_id" gorm:"index"`
	Action    string    `json:"action" gorm:"size:64"`
	Target    string    `json:"target" gorm:"size:128"`
	Details   string    `json:"details,omitempty" gorm:"type:text"`
	CreatedAt time.Time `json:"created_at" gorm:"index"`
}

func (Audit) TableName() string { return "platform_audits" }

type Setting struct {
	Name      string    `json:"name" gorm:"primaryKey;size:80"`
	Value     string    `json:"value" gorm:"type:text"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Setting) TableName() string { return "platform_settings" }

type Announcement struct {
	ID        uint64    `json:"id" gorm:"primaryKey"`
	Title     string    `json:"title" gorm:"size:120"`
	Content   string    `json:"content" gorm:"type:text"`
	CreatedAt time.Time `json:"created_at"`
}

func (Announcement) TableName() string { return "platform_announcements" }

func OpenDatabase(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, errors.New("database connection failed")
	}
	pool, err := db.DB()
	if err != nil {
		return nil, err
	}
	pool.SetMaxOpenConns(10)
	pool.SetMaxIdleConns(3)
	pool.SetConnMaxLifetime(5 * time.Minute)
	return db, nil
}

func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(&User{}, &Token{}, &DailyUsage{}, &Log{}, &Audit{}, &Setting{}, &Announcement{}, &EndpointPolicy{}, &NotificationPreference{}, &Notification{}, &SessionDevice{}); err != nil {
		return err
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		marker := Setting{Name: "migration.profile-and-rate.v1", Value: "complete"}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&marker)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		// Migrate only the previous default once; preserve custom limits on later starts.
		var users []User
		if err := tx.Where("rate_limit = ?", 60).Find(&users).Error; err != nil {
			return err
		}
		for _, user := range users {
			if err := tx.Model(&user).Update("rate_limit", DefaultRateLimit).Error; err != nil {
				return err
			}
			if err := tx.Model(&Token{}).Where("user_id = ? AND rate_limit > ?", user.ID, DefaultRateLimit).Update("rate_limit", DefaultRateLimit).Error; err != nil {
				return err
			}
		}
		var old Setting
		err := tx.Where("name = ?", "notice").First(&old).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if old.Value != "" {
			return tx.Create(&Announcement{Title: "系统公告", Content: old.Value, CreatedAt: old.UpdatedAt}).Error
		}
		return nil
	}); err != nil {
		return err
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		marker := Setting{Name: "migration.daily-quota.v2", Value: "complete"}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&marker)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		var users []User
		if err := tx.Where("daily_quota = ?", 5000).Find(&users).Error; err != nil {
			return err
		}
		for _, user := range users {
			if err := tx.Model(&user).Update("daily_quota", DefaultDailyQuota).Error; err != nil {
				return err
			}
			if err := tx.Model(&Token{}).Where("user_id = ? AND daily_quota > ?", user.ID, DefaultDailyQuota).Update("daily_quota", DefaultDailyQuota).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return migrateOperations(db)
}
func lockForUpdate(tx *gorm.DB) *gorm.DB {
	return tx.Clauses(clause.Locking{Strength: "UPDATE"})
}
