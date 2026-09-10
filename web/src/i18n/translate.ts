import { useTranslation } from 'react-i18next'

import english from '../../../campus/locales/en.json'
import i18n from './config'

export function currentLanguage(): 'en' | 'zh' {
  return i18n.language.startsWith('en') ? 'en' : 'zh'
}

export function dateLocale() {
  return currentLanguage() === 'en' ? 'en-US' : 'zh-CN'
}

// Subscribe at component boundaries; translations also work in event handlers.
export function useLocale() {
  const { i18n: instance } = useTranslation()
  return instance.language
}

const formatted = Object.entries(english)
  .filter(([key]) => /%[ds]/.test(key))
  .map(([key, value]) => ({
    key,
    value,
    pattern: new RegExp(
      `^${key
        .split(/(%[ds]|%%)/)
        .map((part) => {
          if (part === '%d') return '(\\d+)'
          if (part === '%s') return '([\\s\\S]*?)'
          if (part === '%%') return '%'
          return part.replaceAll(/[.*+?^${}()|[\]\\]/g, '\\$&')
        })
        .join('')}$`
    ),
  }))

export function translate(
  source: string | null | undefined,
  variables?: Record<string, unknown>
): string {
  const value = source || ''
  if (currentLanguage() === 'en' && !i18n.exists(value, { lng: 'en' })) {
    for (const entry of formatted) {
      const match = value.match(entry.pattern)
      if (match) {
        let index = 0
        return entry.value.replaceAll(/%[ds]|%%/g, (token) => {
          if (token === '%%') return '%'
          const result = match[++index]
          // Only the action label is system content; names and reasons stay intact.
          return entry.key.startsWith('应用「%s」已%s') && index === 2
            ? translate(result)
            : result
        })
      }
    }
    for (const prefix of ['不支持的查询参数：', '缺少参数：']) {
      if (value.startsWith(prefix)) {
        return translate(prefix) + value.slice(prefix.length)
      }
    }
  }
  return String(i18n.t(value, { ...variables, defaultValue: value }))
}

// Use only for the authored API contract, never for arbitrary school/user data.
export function translateReference<T>(
  value: T,
  locale: string = currentLanguage()
): T {
  if (!locale.startsWith('en')) return value
  if (typeof value === 'string') return translate(value) as T
  if (Array.isArray(value)) {
    return value.map((child) => translateReference(child, locale)) as T
  }
  if (value && typeof value === 'object') {
    return Object.fromEntries(
      Object.entries(value).map(([key, child]) => [
        key,
        translateReference(child, locale),
      ])
    ) as T
  }
  return value
}
