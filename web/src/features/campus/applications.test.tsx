import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/i18n/config'

import { Applications } from './applications'
import { SessionContext } from './session'
import type { Token, User } from './types'

const backend = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('./api', async (original) => ({
  ...(await original<object>()),
  request: backend.request,
}))

const account: User = {
  id: 1,
  username: 'school-user',
  display_name: 'School User',
  role: 'user',
  enabled: true,
  daily_quota: 1000,
  rate_limit: 20,
  created_at: '2026-09-01T00:00:00Z',
  last_login_at: '2026-09-01T00:00:00Z',
}
const application: Token = {
  id: 3,
  user_id: 1,
  name: 'Campus script',
  key_prefix: 'lc_visible',
  scopes: 'timetable:read',
  allowed_ips: '',
  enabled: true,
  daily_quota: 1000,
  rate_limit: 20,
  quota: 100000,
  used_quota: 12,
  expires_at: '2099-09-01T00:00:00Z',
  created_at: '2026-09-01T00:00:00Z',
  last_used_at: null,
}
const catalog = [
  {
    path: '/teaching/timetable',
    scope: 'timetable:read',
    name: 'Timetable',
    parameters: [],
  },
  {
    path: '/teaching/calendar',
    scope: 'calendar:read',
    name: 'Calendar',
    parameters: [],
  },
]
function show(items: Token[] = []) {
  backend.request.mockImplementation((path: string, method?: string) => {
    if (path === '/tokens' && !method) {
      return Promise.resolve({ items, daily_usage: { 3: 12 } })
    }
    if (path === '/catalog') return Promise.resolve(catalog)
    return Promise.reject(new Error('unexpected request'))
  })
  const cache = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={cache}>
      <SessionContext.Provider
        value={{ user: account, csrf_token: 'fixture-csrf' }}
      >
        <Applications />
      </SessionContext.Provider>
    </QueryClientProvider>
  )
}
beforeEach(async () => {
  backend.request.mockReset()
  await i18n.changeLanguage('en')
})
afterEach(cleanup)

describe('developer applications', () => {
  it('preserves the finite draft while unlimited quota is toggled and persists zero as unlimited', async () => {
    const user = userEvent.setup()
    show()
    const create = (
      await screen.findAllByRole('button', { name: 'Create application' })
    )[0]
    await waitFor(() => expect(create).toBeEnabled())
    await user.click(create)
    await user.type(screen.getByLabelText('Application name'), 'Unlimited test')
    expect(screen.getByRole('checkbox', { name: 'Timetables' })).toBeChecked()
    expect(
      screen.getByRole('checkbox', { name: 'Academic calendars' })
    ).toBeChecked()
    const quota = screen.getByLabelText('Total quota')
    await user.clear(quota)
    await user.type(quota, '90000')
    const unlimited = screen.getByRole('checkbox', { name: 'Unlimited' })
    await user.click(unlimited)
    expect(quota).toBeDisabled()
    expect(screen.getByLabelText('Daily quota')).toBeEnabled()
    expect(screen.getByLabelText('Daily quota')).toHaveValue(1000)
    await user.click(unlimited)
    expect(quota).toHaveValue(90000)
    expect(quota).toBeEnabled()
    await user.click(unlimited)
    backend.request.mockResolvedValueOnce({ key: 'lc_unlimited_fixture' })
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(backend.request).toHaveBeenCalledWith(
        '/tokens',
        'POST',
        expect.objectContaining({ quota: 0, daily_quota: 1000, rate_limit: 20 })
      )
    )
  })

  it('restores unlimited quota when editing an existing application', async () => {
    const user = userEvent.setup()
    show([{ ...application, quota: 0, used_quota: 120000 }])
    expect(await screen.findByText('Unlimited', { exact: false })).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Edit Campus script' }))
    expect(screen.getByRole('checkbox', { name: 'Unlimited' })).toBeChecked()
    expect(screen.getByLabelText('Total quota')).toBeDisabled()
  })
  it('keeps the day draft when long term is toggled and submits a permanent application', async () => {
    const user = userEvent.setup()
    show()
    const create = (
      await screen.findAllByRole('button', { name: 'Create application' })
    )[0]
    await waitFor(() => expect(create).toBeEnabled())
    await user.click(create)
    await user.type(
      screen.getByLabelText('Application name'),
      'Permanent script'
    )
    const days = screen.getByLabelText('Validity in days')
    await user.clear(days)
    await user.type(days, '45')
    const longTerm = screen.getByRole('checkbox', { name: 'Long term' })
    await user.click(longTerm)
    expect(days).toBeDisabled()
    expect(days).toHaveValue(45)
    await user.click(longTerm)
    expect(days).toBeEnabled()
    expect(days).toHaveValue(45)
    await user.click(longTerm)
    backend.request.mockResolvedValueOnce({ key: 'lc_test_permanent' })
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByText('lc_test_permanent')).toBeVisible()
    expect(backend.request).toHaveBeenCalledWith(
      '/tokens',
      'POST',
      expect.objectContaining({
        never_expires: true,
        expires_in_days: 45,
        daily_quota: 1000,
        rate_limit: 20,
      })
    )
  })

  it('shows a permanent key as enabled and allows changing it back to a fixed validity', async () => {
    const user = userEvent.setup()
    show([{ ...application, expires_at: null }])
    expect(await screen.findByText('Long term')).toBeVisible()
    expect(screen.getByText('Enabled')).toBeVisible()
    expect(screen.queryByText('Expired')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Edit Campus script' }))
    expect(screen.getByLabelText('Validity in days')).toBeDisabled()
    await user.click(screen.getByRole('checkbox', { name: 'Long term' }))
    expect(screen.getByLabelText('Validity in days')).toHaveValue(90)
    backend.request.mockResolvedValueOnce({ updated: true })
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(backend.request).toHaveBeenCalledWith(
        '/tokens/3',
        'PATCH',
        expect.objectContaining({ never_expires: false, expires_in_days: 90 })
      )
    )
  })

  it('requires a scope, preserves a rejected draft, and only displays a new key until dismissal', async () => {
    const user = userEvent.setup()
    show()
    const create = (
      await screen.findAllByRole('button', { name: 'Create application' })
    )[0]
    await waitFor(() => expect(create).toBeEnabled())
    await user.click(create)
    const dialog = screen.getByRole('dialog')
    await user.type(
      within(dialog).getByLabelText('Application name'),
      'My timetable'
    )
    await user.click(
      within(dialog).getByRole('button', { name: 'Clear selection' })
    )
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'No permissions selected'
    )
    expect(
      backend.request.mock.calls.some(([, method]) => method === 'POST')
    ).toBe(false)
    await user.click(
      within(dialog).getByRole('checkbox', { name: 'Timetables' })
    )
    backend.request.mockRejectedValueOnce(new Error('Please retry'))
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Please retry')
    expect(within(dialog).getByLabelText('Application name')).toHaveValue(
      'My timetable'
    )
    const secret = `lc_${'x'.repeat(43)}`
    backend.request.mockResolvedValueOnce({ key: secret })
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByText(secret)).toBeVisible()
    expect(backend.request).toHaveBeenCalledWith(
      '/tokens',
      'POST',
      expect.objectContaining({
        scopes: ['timetable:read'],
        daily_quota: 1000,
        rate_limit: 20,
      })
    )
    await user.click(screen.getByRole('button', { name: 'Done' }))
    await waitFor(() =>
      expect(screen.queryByText(secret)).not.toBeInTheDocument()
    )
    expect(localStorage.getItem('api_key')).toBeNull()
  })

  it('keeps saved application values when editing is cancelled and confirms deletion', async () => {
    const user = userEvent.setup()
    show([application])
    expect(await screen.findByText('lc_visible••••••••')).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Edit Campus script' }))
    const input = screen.getByLabelText('Application name')
    await user.clear(input)
    await user.type(input, 'Unsaved change')
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.getByRole('heading', { name: 'Campus script' })).toBeVisible()
    expect(
      backend.request.mock.calls.some(([, method]) => method === 'PATCH')
    ).toBe(false)
    await user.click(
      screen.getByRole('button', { name: 'Delete Campus script' })
    )
    expect(screen.getByRole('alertdialog')).toHaveTextContent(
      'Delete application?'
    )
    expect(
      backend.request.mock.calls.some(([, method]) => method === 'DELETE')
    ).toBe(false)
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.getByRole('heading', { name: 'Campus script' })).toBeVisible()
  })
})
