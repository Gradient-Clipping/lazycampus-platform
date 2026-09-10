import { useState, type ReactNode } from 'react'

import {
  PreferencesContext,
  readPreference,
  writePreference,
  sidebarOptions,
  defaultSidebar,
  type SidebarPreferences,
} from './preferences'

export function PreferencesProvider(props: {
  children: ReactNode
  dark: boolean
  changeTheme: (value: boolean) => void
  userId: number
}) {
  const storageKey = `platform-sidebar:v1:${props.userId}`
  const [sidebar, setSidebar] = useState<SidebarPreferences>(() => {
    try {
      const saved = JSON.parse(readPreference(storageKey) || '{}')
      return Object.fromEntries(
        sidebarOptions.map((item) => [item.path, saved[item.path] !== false])
      )
    } catch {
      return defaultSidebar
    }
  })
  return (
    <PreferencesContext.Provider
      value={{
        sidebar,
        dark: props.dark,
        changeTheme: props.changeTheme,
        saveSidebar(value) {
          setSidebar(value)
          writePreference(storageKey, JSON.stringify(value))
        },
      }}
    >
      {props.children}
    </PreferencesContext.Provider>
  )
}
