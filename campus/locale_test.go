package campus

import (
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLanguageNegotiationAndSystemMessages(t *testing.T) {
	for _, tc := range []struct{ path, header, want string }{
		{"/openapi.json", "", "zh"},
		{"/openapi.json", "en-US,en;q=0.9,zh;q=0.8", "en"},
		{"/openapi.json", "zh-CN,en;q=0.8", "zh"},
		{"/openapi.json?lang=en", "zh", "en"},
		{"/openapi.json?lang=zh", "en", "zh"},
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", tc.path, nil)
		c.Request.Header.Set("Accept-Language", tc.header)
		assert.Equal(t, tc.want, requestLanguage(c))
	}
	assert.Equal(t, "Today's custom text 我的应用", translateText("en", "Today's custom text 我的应用"))
	assert.Equal(t, "Application \"我的应用\" has used 800 / 1000 requests, reaching your 80% alert threshold.", translateText("en", "应用「我的应用」已用 800 / 1000 次，达到您设置的 80% 阈值。"))
	assert.Equal(t, "Unsupported query parameter: test", translateText("en", "不支持的查询参数：test"))
	assert.Equal(t, "请求过于频繁，请稍后重试", translateText("zh", "请求过于频繁，请稍后重试"))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/me", nil)
	c.Request.Header.Set("Accept-Language", "en")
	failure(c, 429, "RATE_LIMITED", "请求过于频繁，请稍后重试")
	assert.Equal(t, "en", c.Writer.Header().Get("Content-Language"))
}

func TestEnglishOpenAPIHasCompleteTranslationsAndIdenticalContract(t *testing.T) {
	a, _, _ := testApp(t)
	zh := perform(a, "GET", "/openapi.json?lang=zh", "", "", "")
	en := perform(a, "GET", "/openapi.json?lang=en", "", "", "")
	require.Equal(t, 200, en.Code)
	assert.Equal(t, "en", en.Header().Get("Content-Language"))
	assert.NotRegexp(t, regexp.MustCompile(`\p{Han}`), en.Body.String())
	var chinese, englishSpec map[string]any
	require.NoError(t, json.Unmarshal(zh.Body.Bytes(), &chinese))
	require.NoError(t, json.Unmarshal(en.Body.Bytes(), &englishSpec))
	enPaths := englishSpec["paths"].(map[string]any)
	assert.Len(t, enPaths, 18)
	for path, raw := range chinese["paths"].(map[string]any) {
		before := raw.(map[string]any)["get"].(map[string]any)
		after := enPaths[path].(map[string]any)["get"].(map[string]any)
		for _, key := range []string{"operationId", "x-required-scope", "x-service-status", "x-rate-limit-per-minute", "x-daily-quota", "x-cooldown-seconds"} {
			assert.Equal(t, before[key], after[key], path+" "+key)
		}
		for i, param := range before["parameters"].([]any) {
			original := param.(map[string]any)
			translated := after["parameters"].([]any)[i].(map[string]any)
			for _, key := range []string{"name", "in", "required"} {
				assert.Equal(t, original[key], translated[key])
			}
			originalSchema := original["schema"].(map[string]any)
			translatedSchema := translated["schema"].(map[string]any)
			delete(originalSchema, "description")
			delete(translatedSchema, "description")
			assert.Equal(t, originalSchema, translatedSchema)
		}
	}
	assert.Contains(t, englishSpec["info"].(map[string]any)["description"], "1000 per day")
	// English exports must not mutate shared Chinese schemas.
	again := perform(a, "GET", "/openapi.json?lang=zh", "", "", "")
	assert.Equal(t, zh.Body.String(), again.Body.String())
}
