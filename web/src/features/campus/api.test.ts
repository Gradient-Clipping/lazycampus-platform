import { beforeEach, expect, it, vi } from 'vitest'

const transport = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('axios', () => ({
  default: { create: () => transport, isAxiosError: () => false },
}))

beforeEach(() => {
  vi.resetModules()
  transport.request.mockReset()
})

it('records an existing authenticated session once per document, not on polling', async () => {
  transport.request.mockImplementation(({ url }: { url: string }) =>
    Promise.resolve({
      data: {
        success: true,
        data:
          url === '/me'
            ? {
                user: { id: 1, last_login_at: '2026-09-01T00:00:00Z' },
                csrf_token: 'session-csrf',
              }
            : { last_login_at: '2026-09-11T00:00:00Z' },
      },
    })
  )
  const { getSession } = await import('./api')
  const [first, concurrent] = await Promise.all([getSession(), getSession()])
  expect(first.user.last_login_at).toBe('2026-09-11T00:00:00Z')
  expect(concurrent.user.last_login_at).toBe(first.user.last_login_at)
  await getSession()
  const writes = transport.request.mock.calls.filter(
    ([config]) => config.method === 'POST'
  )
  expect(writes).toHaveLength(1)
  expect(writes[0][0]).toEqual(
    expect.objectContaining({
      url: '/visit',
      headers: { 'X-CSRF-Token': 'session-csrf', 'Accept-Language': 'zh' },
    })
  )
})

it('does not report a platform visit for an unauthenticated session', async () => {
  transport.request.mockRejectedValue(new Error('unauthenticated'))
  const { getSession } = await import('./api')
  await expect(getSession()).rejects.toThrow('unauthenticated')
  expect(transport.request).toHaveBeenCalledTimes(1)
  expect(transport.request.mock.calls[0][0].url).toBe('/me')
})
