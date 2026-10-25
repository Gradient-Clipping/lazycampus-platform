package campus

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

// reference.json is the shared contract for the reader and OpenAPI export.
// It describes the public gateway, not the broader mini-program API.
//
//go:embed reference.json
var referenceJSON []byte

type parameterReference struct {
	Description string         `json:"description"`
	Schema      map[string]any `json:"schema"`
	Example     any            `json:"example,omitempty"`
}

type endpointReference struct {
	Group           string                        `json:"group"`
	Summary         string                        `json:"summary"`
	Notes           []string                      `json:"notes"`
	Related         []string                      `json:"related"`
	Parameters      map[string]parameterReference `json:"parameters"`
	ContentType     string                        `json:"content_type"`
	ResponseSchema  map[string]any                `json:"response_schema"`
	ResponseExample any                           `json:"response_example"`
	RequestSchema   map[string]any                `json:"request_schema,omitempty"`
	RequestExample  any                           `json:"request_example,omitempty"`
	SuccessStatus   int                           `json:"success_status,omitempty"`
}

var reference = func() struct {
	Schemas   map[string]any               `json:"schemas"`
	Endpoints map[string]endpointReference `json:"endpoints"`
} {
	var value struct {
		Schemas   map[string]any               `json:"schemas"`
		Endpoints map[string]endpointReference `json:"endpoints"`
	}
	if err := json.Unmarshal(referenceJSON, &value); err != nil {
		panic(fmt.Errorf("invalid API reference: %w", err))
	}
	return value
}()

// Expand references for the field reader without mutating the shared schemas.
func expandReference(value any) any {
	switch node := value.(type) {
	case map[string]any:
		if name, ok := node["$ref"].(string); ok {
			return expandReference(reference.Schemas[strings.TrimPrefix(name, "#/components/schemas/")])
		}
		result := make(map[string]any, len(node))
		for key, child := range node {
			result[key] = expandReference(child)
		}
		return result
	case []any:
		result := make([]any, len(node))
		for i, child := range node {
			result[i] = expandReference(child)
		}
		return result
	default:
		return value
	}
}

func (a *App) openAPI(c *gin.Context) {
	if c.Query("download") == "1" {
		c.Header("Content-Disposition", `attachment; filename="lazycampus-openapi.json"`)
	}
	tr := func(value string) string { return translateText(requestLanguage(c), value) }
	entries, err := a.catalogEntries()
	if err != nil {
		failure(c, 503, "UNAVAILABLE", "接口文档暂不可用")
		return
	}
	policy, err := a.policy()
	if err != nil {
		failure(c, 503, "UNAVAILABLE", "接口文档暂不可用")
		return
	}
	paths := gin.H{}
	tags := []any{}
	seen := map[string]bool{}
	for _, e := range entries {
		doc := reference.Endpoints[e.Path]
		if !seen[doc.Group] {
			tags = append(tags, gin.H{"name": doc.Group})
			seen[doc.Group] = true
		}
		parameters := []any{}
		for _, p := range e.Parameters {
			detail := doc.Parameters[p.Name]
			parameter := gin.H{"in": e.parameterIn(p), "name": p.Name, "required": p.Required, "description": detail.Description, "schema": detail.Schema}
			if detail.Example != nil {
				parameter["example"] = detail.Example
			}
			parameters = append(parameters, parameter)
		}
		content := gin.H{"schema": doc.ResponseSchema}
		if doc.ResponseExample != nil {
			content["example"] = doc.ResponseExample
		}
		status := "200"
		if doc.SuccessStatus != 0 {
			status = fmt.Sprint(doc.SuccessStatus)
		}
		responses := gin.H{status: gin.H{"description": doc.Summary, "headers": responseHeaders(false), "content": gin.H{doc.ContentType: content}}}
		if doc.ContentType == "application/octet-stream" {
			responses["200"].(gin.H)["content"].(gin.H)["image/*"] = gin.H{"schema": gin.H{"type": "string", "format": "binary"}}
		}
		for status, description := range map[string]string{
			"400": "INVALID_QUERY：参数无效、未知参数、重复参数或超出限制。",
			"401": "INVALID_API_KEY、APP_DISABLED、APP_SUSPENDED、APP_REVOKED、APP_EXPIRED：检查应用状态或更换密钥，不要循环重试。",
			"403": "SCOPE_REQUIRED、IP_NOT_ALLOWED、ACCOUNT_DISABLED、SCHOOL_IDENTITY_REQUIRED：检查权限、IP 白名单、账号与学校身份。校园身份拒绝也会透传对应错误码。",
			"404": "ENDPOINT_NOT_FOUND：路径不存在；CALENDAR_NOT_FOUND、NOTICE_NOT_VISIBLE、GRADE_CLASS_DISTRIBUTION_COURSE_NOT_OWNED 等表示资源不存在或对本人不可见。",
			"429": "RATE_LIMITED、CONCURRENCY_LIMITED、QUOTA_EXHAUSTED 或校园服务独立限流。按 Retry-After 等待；总额度耗尽需调整应用额度，不会每日恢复。",
			"502": "UPSTREAM_UNAVAILABLE、RESPONSE_TOO_LARGE：校园服务暂不可用或响应超过 16 MiB；避免立即密集重试。",
			"503": "ENDPOINT_UNAVAILABLE、POLICY_UNAVAILABLE、LIMITER_UNAVAILABLE、QUOTA_UNAVAILABLE：查看服务状态，稍后重试。",
		} {
			responses[status] = gin.H{"description": description, "headers": responseHeaders(status == "429"), "content": gin.H{"application/json": gin.H{"schema": gin.H{"$ref": "#/components/schemas/APIError"}}}}
		}
		notes := make([]string, len(doc.Notes))
		for i, note := range doc.Notes {
			notes[i] = tr(note)
		}
		description := tr(doc.Summary) + "\n\n" + strings.Join(notes, "\n\n")
		description += fmt.Sprintf(tr("\n\n所需权限：`%s`。当前状态：%s。每用户对此接口的限制：%d 次/分钟，%d 次/日，冷却 %d 秒。还需同时满足账户和应用限制。"), e.Scope, e.Status, e.RateLimit, e.DailyQuota, e.Cooldown)
		if e.Description != "" {
			description += tr("\n\n接口说明：") + tr(e.Description)
		}
		if e.Message != "" {
			description += tr("\n\n服务提示：") + tr(e.Message)
		}
		operation := gin.H{
			"operationId": strings.NewReplacer("/", "_", "-", "_").Replace(strings.TrimPrefix(e.Path, "/")),
			"summary":     e.Name, "description": description, "tags": []string{doc.Group},
			"parameters": parameters, "responses": responses,
			"x-required-scope": e.Scope, "x-service-status": e.Status,
			"x-rate-limit-per-minute": e.RateLimit, "x-daily-quota": e.DailyQuota, "x-cooldown-seconds": e.Cooldown,
		}
		if len(doc.RequestSchema) > 0 {
			operation["requestBody"] = gin.H{"required": true, "content": gin.H{"application/json": gin.H{"schema": doc.RequestSchema, "example": doc.RequestExample}}}
		}
		paths["/v1"+e.Path] = gin.H{strings.ToLower(e.Method): operation}
	}
	schemas := gin.H{}
	for name, schema := range reference.Schemas {
		schemas[name] = schema
	}
	schemas["APIError"] = gin.H{"type": "object", "required": []string{"success", "error"}, "properties": gin.H{
		"success":    gin.H{"type": "boolean", "enum": []bool{false}},
		"error":      gin.H{"type": "object", "required": []string{"code", "message"}, "properties": gin.H{"code": gin.H{"type": "string", "description": "用于程序分支的错误码"}, "message": gin.H{"type": "string", "description": "错误说明"}, "details": gin.H{"description": "可选补充信息，结构依错误码而异"}}},
		"request_id": gin.H{"type": "string", "description": "开放平台生成的请求标识"},
		"requestId":  gin.H{"type": "string", "description": "校园服务透传错误中的请求标识；优先记录 X-Request-Id 响应头"},
	}, "example": gin.H{"success": false, "error": gin.H{"code": "RATE_LIMITED", "message": "请求过于频繁，请稍后重试"}, "request_id": "example-request-id"}}
	description := fmt.Sprintf(tr("所有接口免费。GET 读取本人或已绑定伙伴的授权数据；POST 仅用于本人证明开具，需要独立权限。\n\nBase URL：%s/v1。请求头：Authorization: Bearer <应用密钥>。控制台 SSO Cookie 不代替 API 密钥。\n\n默认账户和新应用每分钟 %d 次、每天 %d 次；同一用户所有应用共享账户额度。每用户并发上限 %d，突发上限 %d 次/秒。账户、应用及接口限制同时生效，每日额度在 UTC 00:00（北京时间 08:00）重置。已接纳请求计入额度，包括校园查询错误；鉴权或网关拒绝不扣调用额度。\n\n与小程序共用北京时间当天首次活动标记。首次调用在响应后后台同步成绩、课表、考试，有缓存时可能先返回旧数据；同步完成后查询读取新数据，失败保留缓存并允许后续活动重试。\n\n仅支持文档列出的路径、查询参数及 JSON 请求体。仅明确标注的接口支持 refresh；教务接口不提供 automatic、waitForSync 或强制刷新。未知或重复查询参数返回 400。page 和 pageSize 最大均为 100，buildingIds 每次最多 5 个。\n\n证明开具 POST 返回 HTTP 202 和 jobId；使用状态接口轮询进度，完成后下载本人 PDF。文件默认保留 24 小时，PDF 最大 20 MiB，其他响应最大 16 MiB。JSON 使用 success/data 和可选 meta，文件返回原始字节。示例数据仅说明结构，标识请从授权查询结果获取。记录 X-Request-Id 排障。在线文档：%s/api-reference。"), a.Config.Origin, policy.RateLimit, policy.DailyQuota, policy.UserConcurrency, policy.Burst, a.Config.Origin)
	writeLocalizedReference(c, gin.H{"openapi": "3.0.3", "info": gin.H{"title": "Lazy Campus 开放 API", "version": "1.0.0", "description": description}, "servers": []any{gin.H{"url": a.Config.Origin, "description": "生产环境"}}, "tags": tags, "security": []any{gin.H{"apiKey": []string{}}}, "paths": paths, "components": gin.H{"schemas": schemas, "securitySchemes": gin.H{"apiKey": gin.H{"type": "http", "scheme": "bearer", "bearerFormat": "lc_...", "description": "在控制台创建应用后获取密钥；仅能访问该应用被授予的权限。"}}}})
}

func responseHeaders(retry bool) gin.H {
	headers := gin.H{
		"X-Request-Id":      gin.H{"description": "平台请求标识；在调用日志中定位请求", "schema": gin.H{"type": "string"}},
		"X-Quota-Remaining": gin.H{"description": "仅完成额度预留后返回：账户当日、应用当日、接口当日与应用剩余总额度中的最小值", "schema": gin.H{"type": "integer"}},
		"RateLimit-Limit":   gin.H{"description": "进入限流检查后返回：账户与应用每分钟上限的较小值，不包含接口限制或并发限制", "schema": gin.H{"type": "integer"}},
	}
	if retry {
		headers["Retry-After"] = gin.H{"description": "建议等待秒数；应用总额度耗尽时等待不会恢复额度，须检查应用用量", "schema": gin.H{"type": "integer", "minimum": 1}}
	}
	return headers
}
