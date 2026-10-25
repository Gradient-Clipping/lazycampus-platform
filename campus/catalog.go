package campus

import (
	"regexp"
	"slices"
	"strings"
)

type Parameter struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Example     string `json:"example,omitempty"`
}
type Endpoint struct {
	Path        string      `json:"path"`
	Name        string      `json:"name"`
	Scope       string      `json:"scope"`
	Description string      `json:"description"`
	Cooldown    int         `json:"cooldown_seconds"`
	Parameters  []Parameter `json:"parameters"`
	Method      string      `json:"method"`
}

var Catalog = []Endpoint{
	{"/teaching/companions", "我的伙伴", "companions:read", "已绑定伙伴列表；绑定关系在小程序管理。", 0, []Parameter{}, "GET"},
	{"/teaching/companions/{id}/timetable", "伙伴课表", "companions:read", "读取已绑定伙伴保存的课表。", 0, []Parameter{{"id", "伙伴列表返回的用户标识", true, "123"}, {"semester", "伙伴课表学期；省略时为默认学期", false, "2026-1"}}, "GET"},
	{"/teaching/companions/{id}/schedule", "伙伴日程", "companions:read", "读取已绑定伙伴保存的自定义课程与日程。", 0, []Parameter{{"id", "伙伴列表返回的用户标识", true, "123"}}, "GET"},
	{"/utilities/electricity/account", "寝室电费", "electricity:read", "本人已绑定寝室的电费余额。", 0, []Parameter{}, "GET"},
	{"/utilities/electricity/details", "电费历史", "electricity:read", "读取已绑定寝室近 30 天用电明细，按需补齐历史。", 5, []Parameter{}, "GET"},
	{"/utilities/laundry/devices", "宿舍洗衣", "laundry:read", "仅查询本人已绑定宿舍的洗衣机和烘干机状态。", 5, []Parameter{}, "GET"},
	{"/campus-documents/certificates", "证明目录", "documents:read", "可开具证明、选项及本人当前任务和文件。", 5, []Parameter{{"refresh", "是否重新同步证明目录，默认 false", false, "false"}}, "GET"},
	{"/campus-documents/certificates/state", "证明开具状态", "documents:read", "查询本人的证明开具进度、结果和文件有效期。", 2, []Parameter{}, "GET"},
	{"/campus-documents/certificates/{id}/options", "证明选项", "documents:read", "读取指定证明的可选项。", 0, []Parameter{{"id", "证明目录返回的标识", true, "certificate-id"}}, "GET"},
	{"/campus-documents/certificates/{id}/issue", "开具证明", "documents:issue", "创建本人证明开具任务，返回任务标识。", 10, []Parameter{{"id", "证明目录返回的标识", true, "certificate-id"}}, "POST"},
	{"/campus-documents/files/{id}", "下载证明", "documents:read", "下载本人有效期内已发布的私有 PDF。", 2, []Parameter{{"id", "开具状态返回的文件 UUID", true, "11111111-2222-4333-8444-555555555555"}}, "GET"},
	{"/teaching/calendar", "教学日历", "calendar:read", "学年日历与可选学年。", 0, []Parameter{{"academicYear", "学年起始年份；省略时为当前学年", false, "2026"}}, "GET"},
	{"/teaching/calendar/image", "校历原图", "calendar:read", "返回校历图片。", 0, []Parameter{{"academicYear", "学年起始年份", false, "2026"}, {"version", "日历元数据中的图片版本", false, ""}}, "GET"},
	{"/teaching/timetable", "我的课表", "timetable:read", "本人课程与学期选项。", 0, []Parameter{{"semester", "接口返回的学期代码", false, "2026-1"}}, "GET"},
	{"/teaching/grades", "我的成绩", "grades:read", "本人已修课程与成绩。", 0, []Parameter{{"academicYear", "学年起始年份", false, "2025"}, {"term", "学期，1、2 或 3", false, "1"}, {"q", "课程关键词", false, ""}, {"sort", "default、academicYear、courseName 或 finalScore", false, "default"}, {"order", "asc 或 desc", false, "desc"}, {"includeUnsuccessful", "是否包含未通过成绩，默认 true", false, "true"}, {"page", "页码", false, "1"}, {"pageSize", "每页条数，最多 100", false, "30"}}, "GET"},
	{"/teaching/grades/class-distribution", "教学班成绩分布", "grades:read", "本人课程的匿名成绩分布。", 0, []Parameter{{"courseId", "本人课程标识", true, ""}, {"academicYear", "学年起始年份", true, "2025"}, {"term", "学期，1、2 或 3", true, "1"}}, "GET"},
	{"/teaching/exams", "我的考试", "exams:read", "本人考试安排。", 0, []Parameter{{"semester", "选项接口返回的学期代码", false, ""}, {"page", "页码", false, "1"}, {"pageSize", "每页条数，最多 100", false, "30"}}, "GET"},
	{"/teaching/exams/options", "考试学期", "exams:read", "可查询的考试学期。", 0, []Parameter{}, "GET"},
	{"/teaching/schedule", "我的日程", "timetable:read", "本人在小程序保存的日程。", 0, []Parameter{}, "GET"},
	{"/teaching/messages", "校园消息", "messages:read", "本人的调课、补课与停课消息。", 0, []Parameter{{"page", "页码", false, "1"}, {"pageSize", "每页条数，最多 100", false, "30"}}, "GET"},
	{"/teaching/notices", "学校通知", "notices:read", "校园通知列表。", 0, []Parameter{{"q", "标题关键词", false, ""}, {"page", "页码", false, "1"}, {"pageSize", "每页条数，最多 100", false, "30"}}, "GET"},
	{"/teaching/notices/detail", "通知详情", "notices:read", "查看通知正文。", 2, []Parameter{{"id", "通知列表返回的标识", true, ""}}, "GET"},
	{"/teaching/notices/attachment", "通知附件", "notices:read", "读取通知正文中的附件。", 3, []Parameter{{"id", "通知标识", true, ""}, {"url", "正文返回的附件 URL", true, ""}}, "GET"},
	{"/teaching/pass-rates", "课程通过率", "grades:read", "本人已修或当前课表课程的统计。", 0, []Parameter{{"courseKey", "接口返回的课程统计键", false, ""}, {"semester", "学期代码", false, ""}, {"timetableCourseId", "课表课程标识", false, ""}}, "GET"},
	{"/teaching/rooms/options", "空教室选项", "rooms:read", "校区、楼栋、节次与可选日期。", 0, []Parameter{{"campusId", "校区编码", false, ""}}, "GET"},
	{"/teaching/rooms", "空教室", "rooms:read", "在选项允许的日期范围内查询空教室。", 10, []Parameter{{"date", "日期 YYYY-MM-DD", true, ""}, {"periods", "节次，逗号分隔", true, "1,2"}, {"campusId", "选项接口返回的校区编码", true, ""}, {"buildingIds", "楼栋编码，最多 5 个", true, ""}, {"page", "页码", false, "1"}, {"pageSize", "每页条数，最多 100", false, "30"}}, "GET"},
	{"/content/feed", "平台公告", "content:read", "校园应用发布的公告与通知。", 0, []Parameter{}, "GET"},
}

func (e Endpoint) parameterIn(p Parameter) string {
	if strings.Contains(e.Path, "{"+p.Name+"}") {
		return "path"
	}
	return "query"
}

var pathID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// Keep policy, quota and logs on the template rather than each user's resource ID.
func requestEndpoint(method, path string) (Endpoint, bool) {
	for _, e := range Catalog {
		if e.Method != method {
			continue
		}
		want, got := strings.Split(e.Path, "/"), strings.Split(path, "/")
		if len(want) != len(got) {
			continue
		}
		matches := true
		for i, part := range want {
			if part == "{id}" {
				matches = matches && pathID.MatchString(got[i])
			} else {
				matches = matches && part == got[i]
			}
		}
		if matches {
			return e, true
		}
	}
	return Endpoint{}, false
}

func endpointFor(path string) (Endpoint, bool) {
	for _, endpoint := range Catalog {
		if endpoint.Path == path {
			return endpoint, true
		}
	}
	return Endpoint{}, false
}
func validScope(scope string) bool {
	return slices.ContainsFunc(Catalog, func(e Endpoint) bool { return e.Scope == scope })
}
