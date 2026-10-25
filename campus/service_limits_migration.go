package campus

import (
	"github.com/Gradient-Clipping/lazycampus-platform/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Upgrade old defaults once, preserving explicit limits, usage and later edits.
func migrateCampusServiceLimits(tx *gorm.DB) error {
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Setting{Name: "migration.campus-services-limits.v1", Value: "complete"})
	if result.Error != nil || result.RowsAffected == 0 {
		return result.Error
	}
	var row Setting
	if err := lockForUpdate(tx).Where("name = ?", "operations.policy.v1").First(&row).Error; err != nil {
		return err
	}
	var policy PlatformPolicy
	if err := common.Unmarshal([]byte(row.Value), &policy); err != nil {
		return err
	}
	if policy.DailyQuota == 1000 {
		policy.DailyQuota = DefaultDailyQuota
		if err := tx.Model(&User{}).Where("inherit_limits = ?", true).Update("daily_quota", policy.DailyQuota).Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE platform_tokens t JOIN platform_users u ON u.id=t.user_id SET t.daily_quota=? WHERE u.inherit_limits=? AND t.daily_quota=?", policy.DailyQuota, true, 1000).Error; err != nil {
			return err
		}
	}
	if policy.RateLimit == 20 {
		policy.RateLimit = DefaultRateLimit
		if err := tx.Model(&User{}).Where("inherit_limits = ?", true).Update("rate_limit", policy.RateLimit).Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE platform_tokens t JOIN platform_users u ON u.id=t.user_id SET t.rate_limit=? WHERE u.inherit_limits=? AND t.rate_limit=?", policy.RateLimit, true, 20).Error; err != nil {
			return err
		}
	}
	raw, err := common.Marshal(policy)
	if err != nil {
		return err
	}
	if err = tx.Model(&row).Update("value", string(raw)).Error; err != nil {
		return err
	}
	if err = tx.Model(&EndpointPolicy{}).Where("daily_quota = ?", 1000).Update("daily_quota", DefaultDailyQuota).Error; err != nil {
		return err
	}
	if err = tx.Model(&EndpointPolicy{}).Where("rate_limit = ?", 20).Update("rate_limit", DefaultRateLimit).Error; err != nil {
		return err
	}
	var siteRow Setting
	if err = tx.Where("name = ?", "operations.site.v1").First(&siteRow).Error; err != nil {
		return err
	}
	var site SiteContent
	if err = common.Unmarshal([]byte(siteRow.Value), &site); err != nil {
		return err
	}
	for i, faq := range site.FAQ {
		if faq.ID == "campus-data-permissions" && faq.Answer == "应用只能读取密钥所属学校用户的数据，且仅能调用授权范围内的接口。开放 API 不支持修改学校数据。" {
			site.FAQ[i].Answer = defaultSite().FAQ[2].Answer
			raw, err = common.Marshal(site)
			if err != nil {
				return err
			}
			if err = tx.Model(&siteRow).Update("value", string(raw)).Error; err != nil {
				return err
			}
		}
	}
	return tx.Model(&EndpointPolicy{}).Where("path IN ?", []string{"/teaching/pass-rates", "/teaching/grades/class-distribution"}).Update("cooldown", 0).Error
}
