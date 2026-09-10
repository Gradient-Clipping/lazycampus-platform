import { createContext, useContext } from 'react'

import type { Session } from './types'

export const SessionContext = createContext<Session | null>(null)
export function useSession(): Session {
  const session = useContext(SessionContext)
  if (!session) throw new Error('Session context missing')
  return session
}
