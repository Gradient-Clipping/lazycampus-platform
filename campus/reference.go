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
			parameter := gin.H{"in": "query", "name": p.Name, "required": p.Required, "description": detail.Description, "schema": detail.Schema}
			if detail.Example != nil {
				parameter["example"] = detail.Example
			}
			parameters = append(parameters, parameter)
		}
		content := gin.H{"schema": doc.ResponseSchema}
		if doc.ResponseExample != nil {
			content["example"] = doc.ResponseExample
		}
		responses := gin.H{"200": gin.H{"description": doc.Summary, "headers": responseHeaders(false), "content": gin.H{doc.ContentType: content}}}
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
		paths["/v1"+e.Path] = gin.H{"get": gin.H{
			"operationId": strings.NewReplacer("/", "_", "-", "_").Replace(strings.TrimPrefix(e.Path, "/")),
			"summary":     e.Name, "description": description, "tags": []string{doc.Group},
			"parameters": parameters, "responses": responses,
			"x-required-scope": e.Scope, "x-service-status": e.Status,
			"x-rate-limit-per-minute": e.RateLimit, "x-daily-quota": e.DailyQuota, "x-cooldown-seconds": e.Cooldown,
		}}
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
	description := fmt.Sprintf(tr("所有接口免费，仅支持 GET，只读取密钥所属学校用户的数据。\n\nBase URL：%s/v1。请求头：Authorization: Bearer <应用密钥>。控制台通过学校 SSO 管理应用，SSO Cookie 不代替 API 密钥。\n\n默认账户和新应用每分钟 %d 次、每天 %d 次；同一用户所有应用共享账户额度。每用户并发上限 %d，突发上限 %d 次/秒。实际限制以账户、应用及接口设置共同约束。分钟窗口从首次请求起持续 60 秒，每日额度在 UTC 00:00（北京时间 08:00）重置。已接纳请求计入额度，包括校园服务返回的错误；鉴权或网关拒绝不扣调用额度，但可能已占用频率窗口。\n\n仅支持各接口列出的查询参数，不支持 refresh、automatic、waitForSync；未知或重复参数返回 400。page 和 pageSize 最大均为 100，buildingIds 每次最多 5 个。\n\nJSON 成功响应为 success/data，部分接口带 meta；图片和附件为二进制。示例数据仅用于说明结构，学期和资源标识请从本人接口结果获取。最多返回 16 MiB。记录 X-Request-Id 用于排障。在线文档：%s/api-reference。"), a.Config.Origin, policy.RateLimit, policy.DailyQuota, policy.UserConcurrency, policy.Burst, a.Config.Origin)
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
