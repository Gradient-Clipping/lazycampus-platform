import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  act,
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import i18n from '@/i18n/config'

import { Devices } from './devices'
import { NotificationPreferences } from './notifications'
import { AdminSettings } from './operations-admin'
import { Logs } from './request-logs'

const backend = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('./api', async (original) => ({
  ...(await original<object>()),
  request: backend.request,
}))
beforeEach(async () => {
  backend.request.mockReset()
  await i18n.changeLanguage('zh')
})
afterEach(cleanup)
function view(component: ReactNode) {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({
          defaultOptions: {
            queries: { retry: false },
            mutations: { retry: false },
          },
        })
      }
    >
      {component}
    </QueryClientProvider>
  )
}
const preferences = {
  user_id: 1,
  email: '',
  in_app: true,
  email_enabled: false,
  quota_enabled: true,
  quota_percent: 80,
  errors_enabled: true,
  error_percent: 30,
  error_minimum: 10,
  rate_enabled: true,
  expiry_enabled: true,
  expiry_days: 7,
  updated_at: '2026-09-01T00:00:00Z',
}

it('switches notification controls into English without losing an email draft', async () => {
  backend.request.mockResolvedValue({
    preferences: { ...preferences },
    email_available: true,
    email_verified: false,
  })
  const user = userEvent.setup()
  view(<NotificationPreferences />)
  const email = await screen.findByLabelText('通知邮箱')
  await user.type(email, 'draft@example.com')
  await act(async () => {
    await i18n.changeLanguage('en')
  })
  expect(screen.getByLabelText('Notification email')).toHaveValue(
    'draft@example.com'
  )
  expect(screen.getByRole('button', { name: 'Verify email' })).toBeVisible()
  expect(
    screen.getByLabelText('Daily or total application quota threshold (%)')
  ).toBeVisible()
  expect(
    screen.getByRole('checkbox', { name: 'Email notification' })
  ).toBeDisabled()
  expect(document.body.textContent?.replace('简体中文', '')).not.toMatch(
    /\p{Script=Han}/u
  )
})

it('keeps email alerts unavailable until an address is saved and verified', async () => {
  const user = userEvent.setup()
  let current = {
    preferences: { ...preferences },
    email_available: true,
    email_verified: false,
  }
  backend.request.mockImplementation(
    async (path: string, method: string, body: Record<string, unknown>) => {
      if (path === '/notification-preferences' && !method) return current
      if (path === '/notification-preferences' && method === 'PUT') {
        current = {
          ...current,
          preferences: { ...current.preferences, ...body },
        }
        return { updated: true }
      }
      if (path.endsWith('/send-verification')) return { sent: true }
      if (path.endsWith('/verify')) {
        current = {
          ...current,
          email_verified: true,
          preferences: {
            ...current.preferences,
            updated_at: '2026-09-01T00:01:00Z',
          },
        }
        return { verified: true }
      }
      throw new Error('Unexpected API')
    }
  )
  view(<NotificationPreferences />)
  expect(
    await screen.findByRole('checkbox', { name: '邮件通知' })
  ).toBeDisabled()
  expect(screen.getByRole('checkbox', { name: '站内通知' })).toBeChecked()
  await user.type(screen.getByLabelText('通知邮箱'), 'student@example.com')
  expect(screen.getByRole('checkbox', { name: '邮件通知' })).toBeDisabled()
  await user.click(screen.getByRole('button', { name: '验证邮箱' }))
  expect(await screen.findByLabelText('邮箱验证码')).toBeVisible()
  expect(backend.request).toHaveBeenCalledWith(
    '/notification-preferences',
    'PUT',
    expect.objectContaining({
      email: 'student@example.com',
      email_enabled: false,
    })
  )
  await user.type(screen.getByLabelText('邮箱验证码'), '123456')
  await user.click(screen.getByRole('button', { name: '确认验证码' }))
  await waitFor(() =>
    expect(screen.getByRole('checkbox', { name: '邮件通知' })).toBeEnabled()
  )
  await user.click(screen.getByRole('checkbox', { name: '邮件通知' }))
  await user.click(screen.getByRole('button', { name: '保存通知偏好' }))
  await waitFor(() =>
    expect(backend.request).toHaveBeenCalledWith(
      '/notification-preferences',
      'PUT',
      expect.objectContaining({ email_enabled: true })
    )
  )
})

it('combines request filters and opens diagnostic details without campus payloads', async () => {
  const user = userEvent.setup()
  const row = {
    id: 9,
    user_id: 1,
    token_id: 3,
    user_name: 'student',
    token_name: '课表脚本',
    endpoint: '/teaching/timetable',
    method: 'GET',
    status: 429,
    admitted: false,
    duration_ms: 2,
    error_code: 'RATE_LIMITED',
    error_message: '调用过于频繁',
    client_ip: '192.0.2.1',
    retry_after: 60,
    request_id: 'request-fixture',
    upstream_request_id: '',
    created_at: '2026-09-11T00:00:00Z',
  }
  backend.request.mockImplementation(async (path: string) => {
    if (path === '/catalog') return [{ path: row.endpoint, name: '我的课表' }]
    if (path === '/logs/9') {
      return {
        log: row,
        user: { username: 'student' },
        application: { name: row.token_name, exists: true },
      }
    }
    if (path.startsWith('/logs?')) {
      return { items: [row], total: 1, page: 1, page_size: 25 }
    }
    throw new Error('Unexpected API')
  })
  view(<Logs />)
  await user.type(screen.getByLabelText('应用编号'), '3')
  await user.selectOptions(screen.getByLabelText('请求状态'), '429')
  await user.click(screen.getByRole('button', { name: '筛选' }))
  await waitFor(() =>
    expect(backend.request).toHaveBeenCalledWith(
      expect.stringContaining('token_id=3&status=429')
    )
  )
  await user.click(await screen.findByRole('button', { name: '查看' }))
  const dialog = await screen.findByRole('dialog')
  expect(await within(dialog).findByText('调用过于频繁')).toBeVisible()
  expect(within(dialog).getByText('60 秒')).toBeVisible()
  expect(within(dialog).getByText('request-fixture')).toBeVisible()
  expect(within(dialog).getByText('否')).toBeVisible()
})

it('preserves a runtime settings draft after a concurrent-update conflict', async () => {
  const user = userEvent.setup()
  const value = {
    daily_quota: 1000,
    rate_limit: 20,
    app_quota: 100000,
    app_days: 90,
    max_apps: 10,
    burst: 10,
    user_concurrency: 3,
    global_concurrency: 20,
  }
  backend.request.mockImplementation(async (path: string, method?: string) => {
    if (method === 'PUT') {
      throw new Error('设置已被其他管理员修改，请刷新后重试')
    }
    if (path.endsWith('/policy')) {
      return { value, updated_at: '2026-09-01T00:00:00Z' }
    }
    return {
      value: {
        name: 'Test',
        description: '',
        logo_url: '/logo.webp',
        help_url: '',
        support_email: '',
        service_status: 'operational',
        service_message: '',
        faq: [],
      },
      updated_at: '2026-09-01T00:00:00Z',
    }
  })
  view(<AdminSettings />)
  const quota = await screen.findByLabelText('默认账户每日额度')
  await user.clear(quota)
  await user.type(quota, '2400')
  await user.click(screen.getByRole('button', { name: '保存并生效' }))
  expect(await screen.findByRole('alert')).toHaveTextContent('其他管理员')
  expect(quota).toHaveValue(2400)
  expect(backend.request).toHaveBeenCalledWith(
    '/admin/settings/policy',
    'PUT',
    expect.objectContaining({
      value: expect.objectContaining({ daily_quota: 2400 }),
      updated_at: '2026-09-01T00:00:00Z',
    })
  )
})

it('revokes other devices while retaining the current device in the refreshed view', async () => {
  const user = userEvent.setup()
  let revoked = false
  const device = {
    id: 'current',
    user_id: 1,
    device: 'Chrome · Windows',
    ip: '192.0.2.1',
    created_at: '2026-09-01T00:00:00Z',
    last_active_at: '2026-09-11T00:00:00Z',
    expires_at: '2026-09-11T08:00:00Z',
  }
  backend.request.mockImplementation(async (path: string, method?: string) => {
    if (path === '/sessions' && !method) {
      return {
        items: revoked
          ? [device]
          : [device, { ...device, id: 'other', device: 'Safari · iOS' }],
        current_id: 'current',
      }
    }
    if (path === '/sessions/revoke-others') {
      revoked = true
      return { revoked: 1, sso_revoked: true }
    }
    throw new Error('Unexpected API')
  })
  view(<Devices />)
  expect(await screen.findByText('Safari · iOS')).toBeVisible()
  await user.click(screen.getByRole('button', { name: '退出其他设备' }))
  const dialog = await screen.findByRole('alertdialog')
  await user.click(within(dialog).getByRole('button', { name: '退出设备' }))
  await waitFor(() =>
    expect(screen.queryByText('Safari · iOS')).not.toBeInTheDocument()
  )
  expect(screen.getByText('当前设备')).toBeVisible()
  expect(backend.request).toHaveBeenCalledWith(
    '/sessions/revoke-others',
    'POST'
  )
})
