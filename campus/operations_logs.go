package campus

import (
	"context"
	"errors"
	"github.com/Gradient-Clipping/lazycampus-platform/common"
	"log"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var safeDiagnostic = regexp.MustCompile(`^[a-zA-Z0-9_.:-]{1,128}$`)
var safeErrorCode = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,79}$`)

func auditDetails(value any) string { raw, _ := common.Marshal(value); return string(raw) }

// Never retain campus response bodies, query values, cookies or API credentials.
func (a *App) recordRequest(c *gin.Context, entry *Log, started time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if entry.UserID == 0 {
		allowed, err := rateScript.Run(ctx, a.Redis, []string{"platform:v1:anonymous-log:" + digest(entry.ClientIP), "platform:v1:anonymous-log:global"}, 10, 60, 300, 60).Int64Slice()
		if err != nil || allowed[0] == 0 {
			return
		}
	}
	entry.Status = c.Writer.Status()
	entry.DurationMS = time.Since(started).Milliseconds()
	if code := c.GetString("errorCode"); code != "" {
		entry.ErrorCode = code
	}
	entry.RetryAfter, _ = strconv.ParseInt(c.Writer.Header().Get("Retry-After"), 10, 64)
	if scope := c.GetString("limitScope"); scope != "" {
		entry.LimitScope = scope
	}
	entry.ErrorMessage = diagnosticMessage(entry.ErrorCode)
	var err error
	if entry.ID == 0 {
		entry.Admitted = false
		err = a.DB.WithContext(ctx).Create(entry).Error
	} else {
		err = a.DB.WithContext(ctx).Model(entry).Updates(map[string]any{"status": entry.Status, "duration_ms": entry.DurationMS, "error_code": entry.ErrorCode, "error_message": entry.ErrorMessage, "upstream_request_id": entry.UpstreamRequestID, "retry_after": entry.RetryAfter, "limit_scope": entry.LimitScope}).Error
	}
	if err != nil {
		log.Print("request diagnostics persistence failed")
	}
}
func diagnosticMessage(code string) string {
	switch code {
	case "":
		return ""
	case "INVALID_API_KEY":
		return "密钥无效"
	case "APP_DISABLED":
		return "应用已停用"
	case "APP_SUSPENDED":
		return "管理员已暂停此应用"
	case "APP_REVOKED":
		return "管理员已撤销此应用授权"
	case "APP_EXPIRED":
		return "应用已到期"
	case "ACCOUNT_DISABLED":
		return "账号不可用"
	case "SCOPE_REQUIRED":
		return "应用没有接口所需权限"
	case "IP_NOT_ALLOWED":
		return "来源 IP 不在应用白名单"
	case "SCHOOL_IDENTITY_REQUIRED", "SCHOOL_IDENTITY_UNAVAILABLE":
		return "学校身份不可用，请在统一登录中检查学校账号"
	case "INVALID_QUERY":
		return "查询参数无效，请对照接口文档检查"
	case "QUOTA_EXHAUSTED":
		return "账户、应用或接口额度已用完"
	case "RATE_LIMITED":
		return "触发每分钟限制、突发限制或接口冷却"
	case "CONCURRENCY_LIMITED":
		return "超过同时进行的请求数限制"
	case "ENDPOINT_UNAVAILABLE":
		return "接口已停用、维护中或权限设置已变化"
	case "ENDPOINT_NOT_FOUND":
		return "接口不存在"
	case "RESPONSE_TOO_LARGE":
		return "响应超过大小限制"
	case "UPSTREAM_UNAVAILABLE":
		return "校园服务暂时不可用"
	case "QUOTA_UNAVAILABLE", "LIMITER_UNAVAILABLE", "POLICY_UNAVAILABLE":
		return "平台服务暂时不可用"
	default:
		return "校园服务拒绝请求，请结合错误码和关联请求编号排查"
	}
}

type logWindow struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

func adminRequest(c *gin.Context) bool { return strings.HasPrefix(c.FullPath(), "/api/admin/") }
func (a *App) filteredLogs(c *gin.Context) (*gorm.DB, logWindow, error) {
	now := time.Now().UTC()
	w := logWindow{Start: now.Add(-7 * 24 * time.Hour), End: now}
	for key, target := range map[string]*time.Time{"from": &w.Start, "to": &w.End} {
		if raw := c.Query(key); raw != "" {
			v, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				return nil, w, errors.New("时间格式无效")
			}
			*target = v.UTC()
		}
	}
	if w.Start.Before(now.Add(-60*24*time.Hour-time.Minute)) || w.End.After(now.Add(time.Minute)) || !w.Start.Before(w.End) || w.End.Sub(w.Start) > 60*24*time.Hour {
		return nil, w, errors.New("请选择最近 60 天内的时间范围")
	}
	q := a.DB.Model(&Log{}).Where("created_at >= ? AND created_at < ?", w.Start, w.End)
	if !adminRequest(c) {
		q = q.Where("user_id = ?", currentUser(c).ID)
	}
	for _, key := range []string{"user_id", "token_id"} {
		if key == "user_id" && !adminRequest(c) {
			continue
		}
		if raw := c.Query(key); raw != "" {
			id, err := strconv.ParseUint(raw, 10, 64)
			if err != nil {
				return nil, w, errors.New("用户或应用编号无效")
			}
			q = q.Where(key+" = ?", id)
		}
	}
	for _, key := range []string{"endpoint", "request_id", "upstream_request_id", "error_code"} {
		if raw := c.Query(key); raw != "" {
			if len(raw) > 160 {
				return nil, w, errors.New("筛选内容过长")
			}
			q = q.Where(key+" = ?", raw)
		}
	}
	status := c.Query("status")
	switch status {
	case "":
	case "error":
		q = q.Where("status >= 400")
	case "success":
		q = q.Where("status >= 200 AND status < 300")
	case "4xx":
		q = q.Where("status >= 400 AND status < 500")
	case "5xx":
		q = q.Where("status >= 500")
	case "rejected":
		q = q.Where("admitted = ?", false)
	default:
		n, err := strconv.Atoi(status)
		if err != nil || n < 100 || n > 599 {
			return nil, w, errors.New("状态筛选无效")
		}
		q = q.Where("status = ?", n)
	}
	for key, op := range map[string]string{"min_ms": ">=", "max_ms": "<="} {
		if raw := c.Query(key); raw != "" {
			v, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || v < 0 || v > 120000 {
				return nil, w, errors.New("耗时筛选无效")
			}
			q = q.Where("duration_ms "+op+" ?", v)
		}
	}
	return q, w, nil
}
func (a *App) logs(c *gin.Context) {
	q, _, err := a.filteredLogs(c)
	if err != nil {
		failure(c, 400, "INVALID_FILTER", err.Error())
		return
	}
	ctx, done := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer done()
	q = q.WithContext(ctx)
	var total int64
	items := []Log{}
	page, size := pagination(c)
	if q.Count(&total).Error != nil || q.Order("id DESC").Offset((page-1)*size).Limit(size).Find(&items).Error != nil {
		failure(c, 503, "UNAVAILABLE", "无法读取日志")
		return
	}
	success(c, gin.H{"items": items, "total": total, "page": page, "page_size": size})
}
func (a *App) logDetail(c *gin.Context) {
	q := a.DB.Where("id = ?", c.Param("id"))
	if !adminRequest(c) {
		q = q.Where("user_id = ?", currentUser(c).ID)
	}
	var item Log
	if q.First(&item).Error != nil {
		failure(c, 404, "NOT_FOUND", "日志不存在")
		return
	}
	var u User
	var token Token
	_ = a.DB.First(&u, item.UserID).Error
	_ = a.DB.First(&token, item.TokenID).Error
	success(c, gin.H{"log": item, "user": gin.H{"id": item.UserID, "username": u.Username, "display_name": u.DisplayName}, "application": gin.H{"id": item.TokenID, "name": token.Name, "exists": token.ID != 0}})
}

type Metrics struct {
	Requests     int64   `json:"requests"`
	Admitted     int64   `json:"admitted"`
	Errors       int64   `json:"errors"`
	Limited      int64   `json:"limited"`
	Rejected     int64   `json:"rejected"`
	ServerErrors int64   `json:"server_errors"`
	AvgMS        float64 `json:"avg_ms"`
	MaxMS        int64   `json:"max_ms"`
}

const metricSQL = "COUNT(*) AS requests, COALESCE(SUM(admitted),0) AS admitted, COALESCE(SUM(status >= 400),0) AS errors, COALESCE(SUM(status = 429),0) AS limited, COALESCE(SUM(NOT admitted),0) AS rejected, COALESCE(SUM(status >= 500),0) AS server_errors, COALESCE(AVG(duration_ms),0) AS avg_ms, COALESCE(MAX(duration_ms),0) AS max_ms"

type metricGroup struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Metrics
}
type metricPoint struct {
	Time string `json:"time"`
	Metrics
}

func (a *App) analytics(c *gin.Context) {
	q, w, err := a.filteredLogs(c)
	if err != nil {
		failure(c, 400, "INVALID_FILTER", err.Error())
		return
	}
	ctx, done := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer done()
	q = q.WithContext(ctx)
	fresh := func() *gorm.DB { return q.Session(&gorm.Session{}) }
	var summary Metrics
	if fresh().Select(metricSQL).Scan(&summary).Error != nil {
		failure(c, 503, "UNAVAILABLE", "无法读取统计")
		return
	}
	var p95 int64
	if summary.Requests > 0 {
		var row Log
		if fresh().Select("duration_ms").Order("duration_ms").Offset(int(math.Ceil(float64(summary.Requests)*.95))-1).Limit(1).Find(&row).Error != nil {
			failure(c, 503, "UNAVAILABLE", "无法读取耗时统计")
			return
		}
		p95 = row.DurationMS
	}
	format := "%Y-%m-%dT%H:00:00Z"
	bucket := "hour"
	if w.End.Sub(w.Start) > 48*time.Hour {
		format = "%Y-%m-%dT00:00:00Z"
		bucket = "day"
	}
	series := []metricPoint{}
	if fresh().Select("DATE_FORMAT(created_at, ?) AS time, "+metricSQL, format).Group("time").Order("time").Scan(&series).Error != nil {
		failure(c, 503, "UNAVAILABLE", "无法读取趋势")
		return
	}
	groups := map[string][]metricGroup{}
	dimensions := map[string]string{"applications": "token_id", "endpoints": "endpoint"}
	if adminRequest(c) {
		dimensions["users"] = "user_id"
	}
	for dimension, column := range dimensions {
		rows := []metricGroup{}
		name := "endpoint"
		if dimension == "applications" {
			name = "MAX(token_name)"
		}
		if dimension == "users" {
			name = "MAX(user_name)"
		}
		if fresh().Select(column+" AS `key`, "+name+" AS name, "+metricSQL).Group(column).Order("requests DESC").Limit(100).Scan(&rows).Error != nil {
			failure(c, 503, "UNAVAILABLE", "无法读取分组统计")
			return
		}
		groups[dimension] = rows
	}
	success(c, gin.H{"summary": summary, "p95_ms": p95, "series": series, "groups": groups, "window": w, "bucket": bucket, "group_limit": 100})
}
