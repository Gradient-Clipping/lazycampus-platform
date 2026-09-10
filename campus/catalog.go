package campus

import "slices"

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
}

var Catalog = []Endpoint{
	{"/teaching/calendar", "教学日历", "calendar:read", "学年日历与可选学年。", 0, []Parameter{{"academicYear", "学年起始年份；省略时为当前学年", false, "2026"}}},
	{"/teaching/calendar/image", "校历原图", "calendar:read", "返回校历图片。", 0, []Parameter{{"academicYear", "学年起始年份", false, "2026"}, {"version", "日历元数据中的图片版本", false, ""}}},
	{"/teaching/timetable", "我的课表", "timetable:read", "本人课程与学期选项。", 0, []Parameter{{"semester", "接口返回的学期代码", false, "2026-1"}}},
	{"/teaching/grades", "我的成绩", "grades:read", "本人已修课程与成绩。", 0, []Parameter{{"academicYear", "学年起始年份", false, "2025"}, {"term", "学期，1、2 或 3", false, "1"}, {"q", "课程关键词", false, ""}, {"sort", "default、academicYear、courseName 或 finalScore", false, "default"}, {"order", "asc 或 desc", false, "desc"}, {"includeUnsuccessful", "是否包含未通过成绩，默认 true", false, "true"}, {"page", "页码", false, "1"}, {"pageSize", "每页条数，最多 100", false, "30"}}},
	{"/teaching/grades/class-distribution", "教学班成绩分布", "grades:read", "本人课程的匿名成绩分布。", 10, []Parameter{{"courseId", "本人课程标识", true, ""}, {"academicYear", "学年起始年份", true, "2025"}, {"term", "学期，1、2 或 3", true, "1"}}},
	{"/teaching/exams", "我的考试", "exams:read", "本人考试安排。", 0, []Parameter{{"semester", "选项接口返回的学期代码", false, ""}, {"page", "页码", false, "1"}, {"pageSize", "每页条数，最多 100", false, "30"}}},
	{"/teaching/exams/options", "考试学期", "exams:read", "可查询的考试学期。", 0, []Parameter{}},
	{"/teaching/schedule", "我的日程", "timetable:read", "本人在小程序保存的日程。", 0, []Parameter{}},
	{"/teaching/messages", "校园消息", "messages:read", "本人的调课、补课与停课消息。", 0, []Parameter{{"page", "页码", false, "1"}, {"pageSize", "每页条数，最多 100", false, "30"}}},
	{"/teaching/notices", "学校通知", "notices:read", "校园通知列表。", 0, []Parameter{{"q", "标题关键词", false, ""}, {"page", "页码", false, "1"}, {"pageSize", "每页条数，最多 100", false, "30"}}},
	{"/teaching/notices/detail", "通知详情", "notices:read", "查看通知正文。", 2, []Parameter{{"id", "通知列表返回的标识", true, ""}}},
	{"/teaching/notices/attachment", "通知附件", "notices:read", "读取通知正文中的附件。", 3, []Parameter{{"id", "通知标识", true, ""}, {"url", "正文返回的附件 URL", true, ""}}},
	{"/teaching/pass-rates", "课程通过率", "grades:read", "本人已修或当前课表课程的统计。", 5, []Parameter{{"courseKey", "接口返回的课程统计键", false, ""}, {"semester", "学期代码", false, ""}, {"timetableCourseId", "课表课程标识", false, ""}}},
	{"/teaching/rooms/options", "空教室选项", "rooms:read", "校区、楼栋、节次与可选日期。", 0, []Parameter{{"campusId", "校区编码", false, ""}}},
	{"/teaching/rooms", "空教室", "rooms:read", "在选项允许的日期范围内查询空教室。", 10, []Parameter{{"date", "日期 YYYY-MM-DD", true, ""}, {"periods", "节次，逗号分隔", true, "1,2"}, {"campusId", "选项接口返回的校区编码", true, ""}, {"buildingIds", "楼栋编码，最多 5 个", true, ""}, {"page", "页码", false, "1"}, {"pageSize", "每页条数，最多 100", false, "30"}}},
	{"/utilities/electricity/account", "寝室电费", "electricity:read", "本人已绑定寝室的电费余额。", 0, []Parameter{}},
	{"/utilities/electricity/buildings", "宿舍楼选项", "electricity:read", "宿舍楼编码与名称。", 0, []Parameter{}},
	{"/content/feed", "平台公告", "content:read", "校园应用发布的公告与通知。", 0, []Parameter{}},
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
