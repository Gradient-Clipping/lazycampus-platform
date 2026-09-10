import { createContext, useContext } from 'react'

export const sidebarOptions = [
  {
    path: '/analytics',
    title: 'Usage analytics',
    detail: 'Usage and request activity',
  },
  {
    path: '/dashboard',
    title: 'Overview',
    detail: 'Usage and request activity',
  },
  {
    path: '/apps',
    title: 'Applications',
    detail: 'Keys, permissions and quotas',
  },
  {
    path: '/api-reference',
    title: 'API reference',
    detail: 'Explore campus APIs',
  },
  {
    path: '/logs',
    title: 'Request logs',
    detail: 'View your application requests',
  },
] as const
export type SidebarPreferences = Record<string, boolean>
export const defaultSidebar = Object.fromEntries(
  sidebarOptions.map((item) => [item.path, true])
)
export function readPreference(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}
export function writePreference(key: string, value: string) {
  try {
    localStorage.setItem(key, value)
  } catch {
    /* Browser storage is optional. */
  }
}
export const PreferencesContext = createContext<{
  sidebar: SidebarPreferences
  saveSidebar: (value: SidebarPreferences) => void
  dark: boolean
  changeTheme: (value: boolean) => void
} | null>(null)
export function usePreferences() {
  const value = useContext(PreferencesContext)
  if (!value) throw new Error('Preferences context missing')
  return value
}
