package campus

import (
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"strconv"
	"strings"
	"time"
)

func (a *App) adminTokens(c *gin.Context) {
	q := a.DB.Model(&Token{})
	if id := c.Query("user_id"); id != "" {
		v, err := strconv.ParseUint(id, 10, 64)
		if err != nil {
			failure(c, 400, "INVALID_FILTER", "用户编号无效")
			return
		}
		q = q.Where("user_id = ?", v)
	}
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		if len(search) > 100 {
			failure(c, 400, "INVALID_FILTER", "搜索内容过长")
			return
		}
		q = q.Where("name LIKE ? OR user_id IN (SELECT id FROM platform_users WHERE username LIKE ? OR display_name LIKE ?)", "%"+search+"%", "%"+search+"%", "%"+search+"%")
	}
	switch c.Query("status") {
	case "active":
		q = q.Where("enabled = ? AND suspended = ? AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)", true, false, time.Now().UTC())
	case "suspended":
		q = q.Where("suspended = ? AND revoked_at IS NULL", true)
	case "revoked":
		q = q.Where("revoked_at IS NOT NULL")
	case "expired":
		q = q.Where("expires_at <= ?", time.Now().UTC())
	case "disabled":
		q = q.Where("enabled = ?", false)
	case "":
	default:
		failure(c, 400, "INVALID_FILTER", "应用状态无效")
		return
	}
	var total int64
	items := []Token{}
	page, size := pagination(c)
	if q.Count(&total).Error != nil || q.Order("id DESC").Offset((page-1)*size).Limit(size).Find(&items).Error != nil {
		failure(c, 503, "UNAVAILABLE", "无法读取应用")
		return
	}
	ids := []uint64{}
	for _, t := range items {
		ids = append(ids, t.UserID)
	}
	users := []User{}
	if len(ids) > 0 && a.DB.Where("id IN ?", ids).Find(&users).Error != nil {
		failure(c, 503, "UNAVAILABLE", "无法读取应用用户")
		return
	}
	success(c, gin.H{"items": items, "users": users, "total": total, "page": page, "page_size": size})
}
func (a *App) regulateToken(c *gin.Context) {
	var input struct {
		Action string `json:"action"`
		Reason string `json:"reason"`
	}
	if c.ShouldBindJSON(&input) != nil || len([]rune(input.Reason)) > 160 || strings.TrimSpace(input.Reason) == "" || (input.Action != "suspend" && input.Action != "resume" && input.Action != "revoke") {
		failure(c, 400, "INVALID_INPUT", "请选择操作并填写原因（最多 160 字）")
		return
	}
	var token Token
	err := a.DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&token, "id = ?", c.Param("id")).Error; err != nil {
			return err
		}
		if token.RevokedAt != nil {
			return errors.New("revoked")
		}
		update := map[string]any{"admin_reason": strings.TrimSpace(input.Reason)}
		switch input.Action {
		case "suspend":
			update["suspended"] = true
		case "resume":
			update["suspended"] = false
		case "revoke":
			update["revoked_at"] = time.Now().UTC()
			update["suspended"] = true
		}
		if err := tx.Model(&token).Updates(update).Error; err != nil {
			return err
		}
		return tx.Create(&Audit{ActorID: currentUser(c).ID, Action: "token." + input.Action, Target: fmt.Sprintf("%d owner=%d", token.ID, token.UserID), Details: input.Reason}).Error
	})
	if err != nil {
		failure(c, 400, "UPDATE_REJECTED", "应用不存在或授权已被撤销")
		return
	}
	a.applicationNotice(token, input.Action, input.Reason)
	success(c, gin.H{"updated": true})
}
