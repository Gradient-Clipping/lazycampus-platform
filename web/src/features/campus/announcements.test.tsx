import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import i18n from '@/i18n/config'

import { AnnouncementBell, AdminAnnouncements } from './announcements'

const backend = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('./api', async (original) => ({
  ...(await original<object>()),
  request: backend.request,
}))
beforeEach(async () => {
  await i18n.changeLanguage('en')
  localStorage.clear()
  backend.request.mockReset()
})
afterEach(cleanup)
function view(component: React.ReactNode) {
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
it('renders announcement content as text and remembers read state for the current account', async () => {
  const user = userEvent.setup()
  backend.request.mockResolvedValue({
    items: [
      {
        id: 8,
        title: 'Campus update',
        content: '<script>unsafe()</script>',
        created_at: '2026-09-11T00:00:00Z',
      },
    ],
  })
  view(<AnnouncementBell userId={21} />)
  await user.click(screen.getByRole('button', { name: 'System announcements' }))
  expect(await screen.findByText('Campus update')).toBeVisible()
  expect(screen.getByText('<script>unsafe()</script>')).toBeVisible()
  expect(document.querySelector('script')).toBeNull()
  await user.click(screen.getByRole('button', { name: 'Mark all read' }))
  expect(localStorage.getItem('platform-announcements-read:v1:21')).toBe('8')
  expect(localStorage.getItem('platform-announcements-read:v1:22')).toBeNull()
  expect(screen.queryByText('New')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Close' }))
  await waitFor(() =>
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  )
})
it('reports a loading error instead of presenting an empty announcement history', async () => {
  backend.request.mockRejectedValue(
    new Error('Announcement service unavailable')
  )
  view(<AnnouncementBell />)
  await userEvent.click(
    screen.getByRole('button', { name: 'System announcements' })
  )
  expect(await screen.findByRole('alert')).toHaveTextContent(
    'Announcement service unavailable'
  )
  expect(screen.queryByText('No notice')).not.toBeInTheDocument()
})
it('keeps an announcement draft after failed publication and clears it only after success', async () => {
  const user = userEvent.setup()
  let reject = true
  backend.request.mockImplementation(async (path: string, method = 'GET') => {
    if (method === 'GET') return { items: [] }
    if (path === '/admin/notice' && reject) throw new Error('Try again')
    return { updated: true }
  })
  view(<AdminAnnouncements />)
  await user.type(screen.getByLabelText('Title'), 'New campus API')
  await user.type(
    screen.getByLabelText('Notice content'),
    'The new endpoint is available.'
  )
  await user.click(screen.getByRole('button', { name: 'Publish notice' }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Publish notice' })).toBeEnabled()
  )
  expect(screen.getByLabelText('Title')).toHaveValue('New campus API')
  reject = false
  await user.click(screen.getByRole('button', { name: 'Publish notice' }))
  await waitFor(() => expect(screen.getByLabelText('Title')).toHaveValue(''))
  expect(screen.getByLabelText('Notice content')).toHaveValue('')
  expect(backend.request).toHaveBeenCalledWith('/admin/notice', 'PUT', {
    title: 'New campus API',
    content: 'The new endpoint is available.',
  })
})
