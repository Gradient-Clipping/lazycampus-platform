import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createRootRoute,
  createRoute,
  createRouter,
  createMemoryHistory,
  RouterProvider,
} from '@tanstack/react-router'
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import i18n from '@/i18n/config'

import { ApiReference } from './reference'
import { requestExample } from './reference-data'
import type { Endpoint } from './types'

const backend = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('./api', () => ({ request: backend.request }))
vi.mock('./session', () => ({
  useSession: () => ({
    user: { rate_limit: 12, daily_quota: 1800 },
    limits: { user_concurrency: 2 },
  }),
}))
const endpoints: Endpoint[] = [
  {
    path: '/teaching/notices/detail',
    name: '通知详情',
    scope: 'notices:read',
    description: '读取正文',
    cooldown_seconds: 2,
    status: 'operational',
    message: '',
    rate_limit: 20,
    daily_quota: 1000,
    updated_at: '2026-09-11T00:00:00Z',
    parameters: [{ name: 'id', required: true, description: '通知 ID' }],
    documentation: {
      group: '消息与通知',
      summary: '读取通知正文与附件信息。',
      notes: ['从列表获取标识。'],
      related: ['/teaching/calendar/image'],
      parameters: {
        id: {
          description: '通知标识',
          schema: { type: 'string' },
          example: 'example-id',
        },
      },
      content_type: 'application/json',
      response_schema: {
        type: 'object',
        properties: {
          data: {
            type: 'object',
            properties: { title: { type: 'string', description: '通知标题' } },
          },
        },
      },
      response_example: { success: true, data: { title: '示例通知' } },
    },
  },
  {
    path: '/teaching/calendar/image',
    name: '校历原图',
    scope: 'calendar:read',
    description: '读取图片',
    cooldown_seconds: 0,
    status: 'operational',
    message: '',
    rate_limit: 20,
    daily_quota: 1000,
    updated_at: '2026-09-11T00:00:00Z',
    parameters: [],
    documentation: {
      group: '教学日历',
      summary: '下载校历图片。',
      notes: ['按 Content-Type 读取。'],
      related: [],
      parameters: {},
      content_type: 'image/*',
      response_schema: { type: 'string', format: 'binary' },
      response_example: null,
    },
  },
]

beforeEach(async () => {
  backend.request.mockResolvedValue(endpoints)
  await i18n.changeLanguage('zh')
})
afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  vi.clearAllMocks()
})
function view(hash = 'start') {
  const root = createRootRoute()
  const route = createRoute({
    getParentRoute: () => root,
    path: '/api-reference',
    component: ApiReference,
  })
  const router = createRouter({
    routeTree: root.addChildren([route]),
    history: createMemoryHistory({
      initialEntries: [`/api-reference#${hash}`],
    }),
  })
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return router
}

it('opens shareable endpoint links, searches scopes and keeps back navigation', async () => {
  const user = userEvent.setup(),
    router = view('api/teaching/notices/detail')
  expect(await screen.findByRole('heading', { name: '通知详情' })).toBeVisible()
  const nav = screen.getByRole('navigation', { name: '文档目录' })
  expect(nav.querySelectorAll('a[aria-current="page"]')).toHaveLength(1)
  await user.type(
    screen.getByRole('textbox', { name: '搜索接口' }),
    'calendar:read'
  )
  expect(
    within(nav).queryByRole('link', { name: /通知详情/ })
  ).not.toBeInTheDocument()
  await user.click(within(nav).getByRole('link', { name: /校历原图/ }))
  expect(await screen.findByRole('heading', { name: '校历原图' })).toBeVisible()
  expect(router.state.location.hash).toBe('api/teaching/calendar/image')
  router.history.back()
  expect(await screen.findByRole('heading', { name: '通知详情' })).toBeVisible()
  await user.click(screen.getByRole('button', { name: '清除' }))
  expect(within(nav).getByRole('link', { name: /通知详情/ })).toHaveAttribute(
    'aria-current',
    'page'
  )
})

it('encodes explorer parameters, shows request diagnostics and never puts the entered key in examples', async () => {
  const user = userEvent.setup()
  view('api/teaching/notices/detail')
  await screen.findByRole('heading', { name: '通知详情' })
  const fetch = vi.fn().mockResolvedValue(
    new Response(
      JSON.stringify({
        success: false,
        error: { code: 'RATE_LIMITED', message: '稍后重试' },
      }),
      {
        status: 429,
        headers: {
          'Content-Type': 'application/json',
          'X-Request-Id': 'test-request-429',
          'Retry-After': '12',
          'X-Quota-Remaining': '0',
        },
      }
    )
  )
  vi.stubGlobal('fetch', fetch)
  await user.type(screen.getByLabelText('应用密钥'), 'lc_TEST_ONLY_SECRET')
  await user.type(screen.getByRole('textbox', { name: 'id *' }), 'ugs:1&中文')
  await user.click(screen.getByRole('button', { name: '发送请求' }))
  expect(await screen.findByText('HTTP 429')).toBeVisible()
  expect(fetch.mock.calls[0][0]).toBe(
    '/v1/teaching/notices/detail?id=ugs%3A1%26%E4%B8%AD%E6%96%87'
  )
  expect(fetch.mock.calls[0][1]).toMatchObject({
    credentials: 'omit',
    headers: { Authorization: 'Bearer lc_TEST_ONLY_SECRET' },
  })
  expect(screen.getByText('剩余额度 0')).toBeVisible()
  expect(screen.getByText('test-request-429')).toBeVisible()
  expect(screen.getByText('Retry-After: 12s')).toBeVisible()
  await user.click(screen.getByRole('button', { name: 'Node.js' }))
  expect(document.querySelector('.reference-code pre')?.textContent).toContain(
    'process.env.LAZYCAMPUS_API_KEY'
  )
  expect(
    document.querySelector('.reference-code pre')?.textContent
  ).not.toContain('lc_TEST_ONLY_SECRET')
})

it('reads binary bytes without decoding them as text and clears keys on endpoint changes', async () => {
  const user = userEvent.setup()
  view('api/teaching/calendar/image')
  await screen.findByRole('heading', { name: '校历原图' })
  const response = {
    headers: new Headers({ 'Content-Type': 'image/webp' }),
    status: 200,
    arrayBuffer: vi
      .fn()
      .mockResolvedValue(new Uint8Array([0xff, 0xfe, 0x80, 0]).buffer),
    text: vi.fn(),
  }
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response))
  await user.type(screen.getByLabelText('应用密钥'), 'lc_BINARY_TEST')
  await user.click(screen.getByRole('button', { name: '发送请求' }))
  await waitFor(() =>
    expect(
      document.querySelector('.response-block pre')?.textContent
    ).toContain('4 字节')
  )
  expect(response.text).not.toHaveBeenCalled()
  expect(document.querySelector('.reference-code pre')?.textContent).toContain(
    '--output response.bin'
  )
  await user.click(
    within(screen.getByRole('navigation', { name: '文档目录' })).getByRole(
      'link',
      { name: /通知详情/ }
    )
  )
  expect(await screen.findByLabelText('应用密钥')).toHaveValue('')
})

it('shows current account limits and produces copyable environment based examples', async () => {
  view('limits')
  expect(await screen.findByText('12 次 / 分钟')).toBeVisible()
  expect(screen.getByText('1,800 次 / 日')).toBeVisible()
  expect(screen.getByText('2 个请求')).toBeVisible()
  const bash = requestExample('/v1/content/feed', false, 'bash')
  expect(bash).toContain('\\\n  -H')
  expect(bash).not.toContain('\n+')
  expect(requestExample('/v1/content/feed', false, 'python')).toContain(
    "os.environ['LAZYCAMPUS_API_KEY']"
  )
})
