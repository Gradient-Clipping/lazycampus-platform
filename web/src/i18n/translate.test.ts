import { afterEach, expect, it } from 'vitest'

import reference from '../../../campus/reference.json'
import { scopeName } from '../features/campus/permissions'
import { requestExample } from '../features/campus/reference-data'
import i18n from './config'
import { translate, translateReference, dateLocale } from './translate'

afterEach(async () => {
  await i18n.changeLanguage('zh')
})

it('translates the complete authored contract without changing the source', async () => {
  const original = JSON.stringify(reference)
  await i18n.changeLanguage('en')
  const translated = translateReference(reference)
  expect(JSON.stringify(translated)).not.toMatch(/\p{Script=Han}/u)
  expect(Object.keys(translated.endpoints)).toEqual(
    Object.keys(reference.endpoints)
  )
  expect(JSON.stringify(reference)).toBe(original)
  await i18n.changeLanguage('zh')
  expect(translateReference(reference)).toBe(reference)
})

it('persists language, formats dates, and translates parameterized messages', async () => {
  await i18n.changeLanguage('en')
  expect(document.documentElement.lang).toBe('en')
  expect(localStorage.getItem('platform-language')).toBe('en')
  expect(document.cookie).toContain('platform_language=en')
  expect(dateLocale()).toBe('en-US')
  expect(translate('个人通知，{{count}} 条未读', { count: 3 })).toBe(
    'Notifications, 3 unread'
  )
  expect(
    translate('应用「我的应用」已用 800 / 1000 次，达到您设置的 80% 阈值。')
  ).toBe(
    'Application "我的应用" has used 800 / 1000 requests, reaching your 80% alert threshold.'
  )
  expect(translate('自定义内容不修改')).toBe('自定义内容不修改')
  expect(scopeName('grades:read')).toBe('Grades and statistics')
  expect(scopeName('unknown:read')).toBe('unknown:read')
  await i18n.changeLanguage('zh')
  expect(dateLocale()).toBe('zh-CN')
  expect(scopeName('grades:read')).toBe('成绩查询')
  expect(document.documentElement.lang).toBe('zh-CN')
})

it('localizes code example messages without exposing a key or changing authentication', async () => {
  await i18n.changeLanguage('en')
  for (const language of ['bash', 'javascript', 'python'] as const) {
    for (const binary of [false, true]) {
      const example = requestExample('/v1/content/feed', binary, language)
      expect(example).not.toMatch(/\p{Script=Han}/u)
      expect(example).toContain('Authorization')
      expect(example).toContain('LAZYCAMPUS_API_KEY')
    }
  }
})
