/* Copyright (C) 2023-2026 QuantumNous. SPDX-License-Identifier: AGPL-3.0-or-later */
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  RouterProvider,
  createRootRoute,
  createRoute,
  createRouter,
} from '@tanstack/react-router'
import { StrictMode } from 'react'
import ReactDOM from 'react-dom/client'
import { Toaster } from 'sonner'

import { Platform } from './features/campus/platform'
import './i18n/config'

import './styles/index.css'
import './styles/experience.css'
import './styles/operations.css'
import './styles/reference.css'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: false, refetchOnWindowFocus: false, staleTime: 30000 },
    mutations: { retry: false },
  },
})
const rootRoute = createRootRoute()
const pages = [
  '/',
  '/dashboard',
  '/analytics',
  '/notifications',
  '/sessions',
  '/status',
  '/admin/overview',
  '/admin/apps',
  '/admin/endpoints',
  '/admin/settings',
  '/profile',
  '/apps',
  '/api-reference',
  '/logs',
  '/account',
  '/admin/users',
  '/admin/logs',
  '/admin/audits',
  '/admin/system',
  '/admin/announcements',
  '/auth-error',
  '/signed-out',
] as const
const routes = pages.map((path) =>
  createRoute({ getParentRoute: () => rootRoute, path, component: Platform })
)
const router = createRouter({
  routeTree: rootRoute.addChildren(routes),
  defaultNotFoundComponent: Platform,
})
declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
const element = document.querySelector('#root')
if (!element) throw new Error('Root element missing')
ReactDOM.createRoot(element).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
      <Toaster richColors position='top-right' />
    </QueryClientProvider>
  </StrictMode>
)
