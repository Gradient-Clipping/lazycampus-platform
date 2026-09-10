package campus

import (
	"github.com/Gradient-Clipping/lazycampus-platform/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// One-time default change. Usage and explicit account limits remain intact;
// subsequent administrator changes are never overwritten on restart.
func migrateDailyQuota1000(tx *gorm.DB) error {
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Setting{Name: "migration.daily-quota.v3", Value: "complete"})
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
	if policy.DailyQuota == 2000 {
		policy.DailyQuota = DefaultDailyQuota
		raw, err := common.Marshal(policy)
		if err != nil {
			return err
		}
		if err = tx.Model(&row).Update("value", string(raw)).Error; err != nil {
			return err
		}
		if err = tx.Model(&User{}).Where("inherit_limits = ?", true).Update("daily_quota", policy.DailyQuota).Error; err != nil {
			return err
		}
		if err = tx.Exec("UPDATE platform_tokens t JOIN platform_users u ON u.id=t.user_id SET t.daily_quota=? WHERE u.inherit_limits=? AND t.daily_quota > ?", policy.DailyQuota, true, policy.DailyQuota).Error; err != nil {
			return err
		}
	}
	return tx.Model(&EndpointPolicy{}).Where("daily_quota = ?", 2000).Update("daily_quota", DefaultDailyQuota).Error
}
