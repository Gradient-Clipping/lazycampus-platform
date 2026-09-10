import axios from 'axios'

import { translate, dateLocale, currentLanguage } from '@/i18n/translate'

import type { Session } from './types'

export const api = axios.create({
  baseURL: '/api',
  withCredentials: true,
  timeout: 20000,
})
let csrfToken = ''
// Record an authenticated document opening once, including a restored SSO session.
// React Query refetches and client-side navigation must not turn into new logins.
const visits = new Map<number, Promise<{ last_login_at: string }>>()
export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public code: string
  ) {
    super(message)
  }
}
export async function request<T>(
  path: string,
  method = 'GET',
  data?: unknown
): Promise<T> {
  try {
    const response = await api.request<{
      success: boolean
      data: T
      error?: { message: string; code: string }
    }>({
      url: path,
      method,
      data,
      headers: {
        'Accept-Language': currentLanguage(),
        ...(method === 'GET' ? {} : { 'X-CSRF-Token': csrfToken }),
      },
    })
    if (!response.data.success) {
      throw new ApiError(
        response.data.error?.message || translate('请求失败'),
        response.status,
        response.data.error?.code || 'ERROR'
      )
    }
    return response.data.data
  } catch (error) {
    if (axios.isAxiosError(error)) {
      throw new ApiError(
        String(
          error.response?.data?.error?.message ||
            translate('请求失败，请稍后重试')
        ),
        error.response?.status || 503,
        String(error.response?.data?.error?.code || 'UNAVAILABLE')
      )
    }
    throw error
  }
}
export async function getSession(): Promise<Session> {
  const session = await request<Session>('/me')
  csrfToken = session.csrf_token
  let visit = visits.get(session.user.id)
  if (!visit) {
    visit = request<{ last_login_at: string }>('/visit', 'POST').catch(
      (error: unknown) => {
        visits.delete(session.user.id)
        throw error
      }
    )
    visits.set(session.user.id, visit)
  }
  const recorded = await visit
  // A different tab may have opened the platform more recently.
  if (
    Date.parse(recorded.last_login_at) > Date.parse(session.user.last_login_at)
  ) {
    session.user.last_login_at = recorded.last_login_at
  }
  return session
}
export function formatDate(value: string | null): string {
  if (!value || value.startsWith('0001-')) return '—'
  return new Date(value).toLocaleString(dateLocale(), {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export function formatLoginDate(value: string): string {
  if (!value || value.startsWith('0001-')) return '—'
  return new Date(value).toLocaleString(dateLocale(), {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  })
}
