package campus

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Gradient-Clipping/lazycampus-platform/common"
	"github.com/gin-gonic/gin"
)

func (a *App) gateway(c *gin.Context) {
	started := time.Now()
	entry := Log{RequestID: c.GetString("requestID"), Endpoint: "/unknown", Method: c.Request.Method, ClientIP: c.ClientIP()}
	defer func() { a.recordRequest(c, &entry, started) }()
	endpoint, ok := requestEndpoint(c.Request.Method, c.Param("path"))
	if !ok {
		failure(c, 404, "ENDPOINT_NOT_FOUND", "接口不存在")
		return
	}
	entry.Endpoint = endpoint.Path
	token, user, ok := a.authenticateAPI(c, &entry)
	if !ok {
		return
	}
	var endpointPolicy EndpointPolicy
	if a.DB.First(&endpointPolicy, "path = ?", endpoint.Path).Error != nil {
		failure(c, 503, "POLICY_UNAVAILABLE", "接口设置暂时不可用")
		return
	}
	if endpointPolicy.Status == "disabled" || endpointPolicy.Status == "maintenance" {
		failure(c, 503, "ENDPOINT_UNAVAILABLE", "接口已停用或正在维护")
		return
	}
	endpoint.Scope, endpoint.Cooldown = endpointPolicy.Scope, endpointPolicy.Cooldown
	if !slices.Contains(strings.Fields(token.Scopes), endpoint.Scope) {
		failure(c, 403, "SCOPE_REQUIRED", "应用没有此接口的权限")
		return
	}
	if err := validateQuery(c.Request.URL.Query(), endpoint); err != nil {
		failure(c, 400, "INVALID_QUERY", err.Error())
		return
	}
	body, err := gatewayBody(c, endpoint)
	if err != nil {
		failure(c, 400, "INVALID_BODY", err.Error())
		return
	}
	release, ok := a.apiRate(c, user, token, endpoint, endpointPolicy)
	if !ok {
		return
	}
	defer release()
	remaining, err := a.reserve(c.Request.Context(), user.ID, token.ID, &entry, token)
	if errors.Is(err, errQuota) {
		now := time.Now().UTC()
		seconds := int64(time.Until(time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)).Seconds()) + 1
		c.Header("Retry-After", strconv.FormatInt(seconds, 10))
		failure(c, 429, "QUOTA_EXHAUSTED", "调用额度已用完")
		return
	}
	if errors.Is(err, errDisabled) {
		failure(c, 401, "INVALID_API_KEY", "应用密钥已失效")
		return
	}
	if errors.Is(err, errEndpoint) {
		failure(c, 503, "ENDPOINT_UNAVAILABLE", "接口设置已变化，请稍后重试")
		return
	}
	if err != nil {
		failure(c, 503, "QUOTA_UNAVAILABLE", "服务暂时不可用")
		return
	}
	c.Header("X-Quota-Remaining", strconv.FormatInt(remaining, 10))
	path := "/internal/platform/v1" + c.Param("path")
	query := c.Request.URL.Query().Encode()
	target := path
	if query != "" {
		target += "?" + query
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 85*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, c.Request.Method, a.Config.CampusURL+target, bytes.NewReader(body))
	if err != nil {
		failure(c, 502, "UPSTREAM_UNAVAILABLE", "校园服务暂时不可用")
		return
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := randomKey(24)
	canonical := strings.Join([]string{c.Request.Method, target, timestamp, nonce, user.IdentityID, digest(string(body))}, "\n")
	mac := hmac.New(sha256.New, []byte(a.Config.CampusSecret))
	mac.Write([]byte(canonical))
	request.Header.Set("X-Platform-Timestamp", timestamp)
	request.Header.Set("X-Platform-Nonce", nonce)
	request.Header.Set("X-Platform-Identity", user.IdentityID)
	request.Header.Set("X-Platform-Signature", hex.EncodeToString(mac.Sum(nil)))
	request.Header.Set("X-Request-Id", entry.RequestID)
	if c.Request.Method == "POST" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := a.HTTP.Do(request)
	if err != nil {
		failure(c, 502, "UPSTREAM_UNAVAILABLE", "校园服务暂时不可用，请稍后重试")
		return
	}
	defer response.Body.Close()
	if safeDiagnostic.MatchString(response.Header.Get("X-Request-Id")) {
		entry.UpstreamRequestID = response.Header.Get("X-Request-Id")
	}
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		failure(c, 502, "UPSTREAM_UNAVAILABLE", "校园服务暂时不可用")
		return
	}
	if retry := response.Header.Get("Retry-After"); retry != "" {
		c.Header("Retry-After", retry)
	}
	contentType := response.Header.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(contentType)
	// Notifications may contain HTML; serve all non-JSON/image content as downloads.
	pdf := endpoint.Path == "/campus-documents/files/{id}" && mediaType == "application/pdf"
	if mediaType != "application/json" && !strings.HasPrefix(mediaType, "image/") && !pdf {
		contentType = "application/octet-stream"
		c.Header("Content-Disposition", "attachment")
	}
	maxBytes := int64(16 * 1024 * 1024)
	if pdf {
		maxBytes = 20 * 1024 * 1024
		c.Header("Content-Disposition", "attachment")
	}
	body, err = io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil || int64(len(body)) > maxBytes {
		failure(c, 502, "RESPONSE_TOO_LARGE", "响应超出大小限制")
		return
	}
	if response.StatusCode >= 500 {
		failure(c, 502, "UPSTREAM_UNAVAILABLE", "校园服务暂时不可用，请稍后重试")
		return
	}
	if response.StatusCode >= 400 {
		entry.ErrorCode = "UPSTREAM_REJECTED"
		var diagnostic struct {
			Code  string `json:"code"`
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if common.Unmarshal(body, &diagnostic) == nil {
			if safeErrorCode.MatchString(diagnostic.Code) {
				entry.ErrorCode = diagnostic.Code
			} else if safeErrorCode.MatchString(diagnostic.Error.Code) {
				entry.ErrorCode = diagnostic.Error.Code
			}
		}
	}
	if endpoint.Path == "/teaching/calendar" && mediaType == "application/json" && response.StatusCode == 200 {
		var payload map[string]any
		if common.Unmarshal(body, &payload) != nil {
			failure(c, 502, "UPSTREAM_UNAVAILABLE", "校园服务响应无效")
			return
		}
		rewriteCalendarLinks(payload)
		body, _ = common.Marshal(payload)
	}
	c.Data(response.StatusCode, contentType, body)
}

func rewriteCalendarLinks(value any) {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			if link, ok := child.(string); ok && key == "imageUrl" && strings.HasPrefix(link, "/api/v1/teaching/calendar/image?") {
				node[key] = strings.TrimPrefix(link, "/api")
			} else {
				rewriteCalendarLinks(child)
			}
		}
	case []any:
		for _, child := range node {
			rewriteCalendarLinks(child)
		}
	}
}

func allowedIP(rules, address string) bool {
	if strings.TrimSpace(rules) == "" {
		return true
	}
	ip, err := netip.ParseAddr(address)
	if err != nil {
		return false
	}
	ip = ip.Unmap()
	for _, rule := range strings.Fields(rules) {
		if prefix, err := netip.ParsePrefix(rule); err == nil && prefix.Contains(ip) {
			return true
		}
		if candidate, err := netip.ParseAddr(rule); err == nil && candidate.Unmap() == ip {
			return true
		}
	}
	return false
}

func validateQuery(query url.Values, e Endpoint) error {
	if len(query.Encode()) > 4096 {
		return errors.New("查询参数过长")
	}
	for key, values := range query {
		if len(values) != 1 || len(values[0]) > 1024 {
			return errors.New("查询参数无效")
		}
		if !slices.ContainsFunc(e.Parameters, func(p Parameter) bool { return p.Name == key && e.parameterIn(p) == "query" }) {
			return errors.New("不支持的查询参数：" + key)
		}
	}
	for _, p := range e.Parameters {
		if p.Required && e.parameterIn(p) == "query" && query.Get(p.Name) == "" {
			return errors.New("缺少参数：" + p.Name)
		}
	}
	for _, name := range []string{"page", "pageSize"} {
		if value := query.Get(name); value != "" {
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 || (name == "pageSize" && n > 100) || (name == "page" && n > 100) {
				return errors.New("分页参数超出允许范围")
			}
		}
	}
	if ids := query.Get("buildingIds"); ids != "" && len(strings.Split(ids, ",")) > 5 {
		return errors.New("每次最多查询 5 栋楼")
	}
	return nil
}

func gatewayBody(c *gin.Context, e Endpoint) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 32*1024+1))
	if err != nil || len(raw) > 32*1024 {
		return nil, errors.New("请求体超出大小限制")
	}
	if e.Method != "POST" {
		if len(raw) != 0 {
			return nil, errors.New("GET 请求不接受请求体")
		}
		return nil, nil
	}
	mediaType, _, _ := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if mediaType != "application/json" {
		return nil, errors.New("请求体必须为 application/json")
	}
	var input struct {
		Selected json.RawMessage `json:"selected"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("证明选项必须为 JSON 对象，最多 20 项")
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, errors.New("请求体必须为 JSON 对象")
	}
	selected := map[string]any{}
	if len(input.Selected) > 0 && (json.Unmarshal(input.Selected, &selected) != nil || selected == nil) {
		return nil, errors.New("证明选项必须为 JSON 对象，最多 20 项")
	}
	if len(selected) > 20 {
		return nil, errors.New("证明选项必须为 JSON 对象，最多 20 项")
	}
	for key, rawValue := range selected {
		value, ok := rawValue.(string)
		if !ok {
			return nil, errors.New("证明选项必须为 JSON 对象，最多 20 项")
		}
		if len([]rune(key)) > 100 || len([]rune(value)) > 100 {
			return nil, errors.New("证明选项最多 100 字符")
		}
	}
	return raw, nil
}

// Shared by REST, GraphQL and MCP; credentials are revalidated for every data call.
func (a *App) authenticateAPI(c *gin.Context, entry *Log) (Token, User, bool) {
	key, bearer := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
	if !bearer || !strings.HasPrefix(key, "lc_") || len(key) != 46 {
		failure(c, 401, "INVALID_API_KEY", "应用密钥无效")
		return Token{}, User{}, false
	}
	var token Token
	var user User
	if a.DB.Where("key_hash = ?", digest(key)).First(&token).Error != nil {
		failure(c, 401, "INVALID_API_KEY", "应用密钥无效或已过期")
		return Token{}, User{}, false
	}
	entry.UserID, entry.TokenID, entry.TokenName = token.UserID, token.ID, token.Name
	if !token.Enabled || token.Suspended || token.RevokedAt != nil || (token.ExpiresAt != nil && !token.ExpiresAt.After(time.Now())) {
		code := "APP_DISABLED"
		if token.Suspended {
			code = "APP_SUSPENDED"
		} else if token.RevokedAt != nil {
			code = "APP_REVOKED"
		} else if token.ExpiresAt != nil && !token.ExpiresAt.After(time.Now()) {
			code = "APP_EXPIRED"
		}
		failure(c, 401, code, "应用已停用、撤销或到期")
		return Token{}, User{}, false
	}
	if a.DB.First(&user, token.UserID).Error != nil || !user.Enabled {
		failure(c, 403, "ACCOUNT_DISABLED", "当前账号不可用")
		return Token{}, User{}, false
	}
	entry.UserName = user.Username
	if !allowedIP(token.AllowedIPs, c.ClientIP()) {
		failure(c, 403, "IP_NOT_ALLOWED", "当前 IP 不在允许范围内")
		return Token{}, User{}, false
	}
	if !identityID.MatchString(user.IdentityID) {
		failure(c, 403, "SCHOOL_IDENTITY_REQUIRED", "此接口需要学校身份")
		return Token{}, User{}, false
	}
	return token, user, true
}
