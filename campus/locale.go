package campus

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/text/language"
)

// English resources are shared with the console. Chinese is the canonical source
// for authored documentation and stored system messages, never for user data.
//
//go:embed locales/en.json
var englishJSON []byte

var english = func() map[string]string {
	var result map[string]string
	if err := json.Unmarshal(englishJSON, &result); err != nil {
		panic(err)
	}
	return result
}()

var formatToken = regexp.MustCompile(`%[ds]|%%`)

type translatedFormat struct {
	key, value string
	pattern    *regexp.Regexp
}

var translatedFormats = func() []translatedFormat {
	keys := []string{}
	for key := range english {
		if strings.Contains(key, "%d") || strings.Contains(key, "%s") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	result := []translatedFormat{}
	for _, key := range keys {
		var pattern strings.Builder
		pattern.WriteString("(?s)^")
		last := 0
		for _, match := range formatToken.FindAllStringIndex(key, -1) {
			pattern.WriteString(regexp.QuoteMeta(key[last:match[0]]))
			switch key[match[0]:match[1]] {
			case "%d":
				pattern.WriteString(`(\d+)`)
			case "%s":
				pattern.WriteString(`(.*?)`)
			case "%%":
				pattern.WriteString("%")
			}
			last = match[1]
		}
		pattern.WriteString(regexp.QuoteMeta(key[last:]) + "$")
		result = append(result, translatedFormat{key, english[key], regexp.MustCompile(pattern.String())})
	}
	return result
}()

func requestLanguage(c *gin.Context) string {
	if locale := c.Query("lang"); locale == "en" || locale == "zh" {
		return locale
	}
	header := c.GetHeader("Accept-Language")
	if header == "" {
		if cookie, err := c.Cookie("platform_language"); err == nil && cookie == "en" {
			return "en"
		}
		return "zh"
	}
	matcher := language.NewMatcher([]language.Tag{language.Chinese, language.English})
	_, index := language.MatchStrings(matcher, header)
	if index == 1 {
		return "en"
	}
	return "zh"
}

func translateText(locale, value string) string {
	if locale != "en" {
		return value
	}
	if translated, ok := english[value]; ok {
		return translated
	}
	for _, entry := range translatedFormats {
		if match := entry.pattern.FindStringSubmatch(value); match != nil {
			index := 0
			return formatToken.ReplaceAllStringFunc(entry.value, func(token string) string {
				if token == "%%" {
					return "%"
				}
				index++
				result := match[index]
				if strings.HasPrefix(entry.key, "应用「%s」已%s") && index == 2 {
					result = translateText(locale, result)
				}
				return result
			})
		}
	}
	for _, prefix := range []string{"不支持的查询参数：", "缺少参数："} {
		if strings.HasPrefix(value, prefix) {
			return english[prefix] + strings.TrimPrefix(value, prefix)
		}
	}
	return value
}

// Only call for the authored OpenAPI document, never school or user responses.
func writeLocalizedReference(c *gin.Context, value any) {
	locale := requestLanguage(c)
	c.Header("Content-Language", locale)
	c.Header("Vary", "Accept-Language, Cookie")
	if locale == "zh" {
		c.JSON(200, value)
		return
	}
	raw, err := json.Marshal(value)
	if err != nil {
		failure(c, 503, "UNAVAILABLE", "接口文档暂不可用")
		return
	}
	var tree any
	if err = json.Unmarshal(raw, &tree); err != nil {
		failure(c, 503, "UNAVAILABLE", "接口文档暂不可用")
		return
	}
	var walk func(any) any
	walk = func(node any) any {
		switch item := node.(type) {
		case string:
			return translateText(locale, item)
		case []any:
			for i := range item {
				item[i] = walk(item[i])
			}
		case map[string]any:
			for key := range item {
				item[key] = walk(item[key])
			}
		}
		return node
	}
	c.JSON(200, walk(tree))
}
