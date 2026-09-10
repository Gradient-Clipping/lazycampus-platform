import { translate } from '@/i18n/translate'

const names: Record<string, string> = {
  'timetable:read': '课表查询',
  'grades:read': '成绩查询',
  'exams:read': '考试查询',
  'calendar:read': '校历查询',
  'rooms:read': '空教室查询',
  'messages:read': '校园消息查询',
  'notices:read': '学校通知查询',
  'electricity:read': '寝室电费查询',
  'content:read': '校园公告查询',
}

export function scopeName(scope: string) {
  return translate(names[scope] || scope)
}

export function scopeNames(scopes: string) {
  return scopes.split(' ').filter(Boolean).map(scopeName).join(' · ')
}
