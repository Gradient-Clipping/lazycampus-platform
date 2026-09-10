import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'

import english from '../../../campus/locales/en.json'
import zh from './locales/zh.json'

function savedLanguage() {
  try {
    return localStorage.getItem('platform-language') === 'en' ? 'en' : 'zh'
  } catch {
    return 'zh'
  }
}

function persistLanguage(language: string) {
  if (typeof document === 'undefined') return
  const locale = language.startsWith('en') ? 'en' : 'zh'
  document.documentElement.lang = locale === 'en' ? 'en' : 'zh-CN'
  try {
    localStorage.setItem('platform-language', locale)
  } catch {
    /* Storage may be disabled in a private browser. */
  }
  document.cookie = `platform_language=${locale}; Path=/; Max-Age=31536000; SameSite=Lax${location.protocol === 'https:' ? '; Secure' : ''}`
}
i18n.on('languageChanged', persistLanguage)

void i18n.use(initReactI18next).init({
  lng: savedLanguage(),
  fallbackLng: 'en',
  keySeparator: false,
  nsSeparator: false,
  resources: {
    zh: {
      translation: {
        ...Object.fromEntries(Object.keys(english).map((key) => [key, key])),
        ...zh,
      },
    },
    en: {
      translation: {
        ...Object.fromEntries(Object.keys(zh).map((key) => [key, key])),
        ...english,
      },
    },
  },
  interpolation: { escapeValue: false },
})
export default i18n
