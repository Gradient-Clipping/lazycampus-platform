import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useRouterState } from '@tanstack/react-router'
import {
  Activity,
  BarChart3,
  Monitor,
  ArrowUpRight,
  BookOpen,
  Box,
  ClipboardList,
  LayoutDashboard,
  LogOut,
  Megaphone,
  Menu,
  Moon,
  PanelLeftClose,
  Settings2,
  ShieldCheck,
  Sun,
  UserRound,
  Users,
  X,
} from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import { translate } from '@/i18n/translate'

import { UsageAnalytics } from './analytics'
import { AnnouncementBell, AdminAnnouncements } from './announcements'
import { ApiError, getSession, request } from './api'
import { Applications } from './applications'
import { Devices } from './devices'
import { Home } from './home'
import { AdminUsers, AdminSystem, AuditLogs } from './management'
import { Notifications, PersonalBell } from './notifications'
import {
  AdminApplications,
  AdminEndpoints,
  AdminSettings,
} from './operations-admin'
import { useSite } from './operations-data'
import { Overview } from './overview'
import { readPreference, usePreferences, writePreference } from './preferences'
import { PreferencesProvider } from './preferences-provider'
import { Profile } from './profile'
import { ApiReference } from './reference'
import { Logs } from './request-logs'
import { SessionContext } from './session'
import { ErrorState, Loading } from './shared'
import { ServiceStatusPage } from './site-content'
import type { User } from './types'

const navigation = [
  { to: '/dashboard', label: 'Overview', icon: LayoutDashboard },
  { to: '/analytics', label: 'Usage analytics', icon: BarChart3 },
  { to: '/apps', label: 'Applications', icon: Box },
  { to: '/api-reference', label: 'API reference', icon: BookOpen },
  { to: '/logs', label: 'Request logs', icon: Activity },
] as const
const administration = [
  { to: '/admin/overview', label: 'Platform analytics', icon: LayoutDashboard },
  { to: '/admin/apps', label: 'Application oversight', icon: Box },
  { to: '/admin/endpoints', label: 'Endpoint management', icon: BookOpen },
  { to: '/admin/users', label: 'Users', icon: Users },
  { to: '/admin/logs', label: 'Request logs', icon: ClipboardList },
  { to: '/admin/audits', label: 'Audit logs', icon: ShieldCheck },
  {
    to: '/admin/announcements',
    label: 'System announcements',
    icon: Megaphone,
  },
  { to: '/admin/system', label: 'System', icon: Settings2 },
  { to: '/admin/settings', label: 'Site and quotas', icon: Settings2 },
] as const

export function Platform() {
  const { t } = useTranslation()
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const returnTo = useRouterState({
    select: (state) => state.location.href,
  })
  const isHome = pathname === '/' || pathname === '/status'
  const site = useSite()
  useEffect(() => {
    if (site.data) {
      document.title = translate('{{v0}} · 开放平台', {
        v0: site.data.site.name,
      })
    }
  }, [site.data, t])
  const isAuthMessage = pathname === '/auth-error' || pathname === '/signed-out'
  const session = useQuery({
    queryKey: ['session'],
    queryFn: getSession,
    enabled: !isAuthMessage,
    staleTime: 60000,
    refetchInterval: 4 * 60000,
    retry: (count, error) =>
      count < 1 && error instanceof ApiError && error.code === 'SSO_REFRESHING',
  })
  const cache = useQueryClient()
  const [logoutOpen, setLogoutOpen] = useState(false)
  const [signingOut, setSigningOut] = useState(false)
  const [dark, setDark] = useState(
    () => readPreference('platform-theme') === 'dark'
  )
  useEffect(() => {
    document.documentElement.classList.toggle('dark', dark)
    writePreference('platform-theme', dark ? 'dark' : 'light')
  }, [dark])
  useEffect(() => {
    if (
      !isHome &&
      !isAuthMessage &&
      session.error instanceof ApiError &&
      session.error.status === 401
    ) {
      window.location.replace(
        `/auth/start?return_to=${encodeURIComponent(returnTo)}`
      )
    }
  }, [isHome, isAuthMessage, session.error, returnTo])
  async function signOut() {
    setSigningOut(true)
    try {
      const result = await request<{ url: string }>('/logout', 'POST')
      cache.clear()
      window.location.assign(result.url)
    } catch (error) {
      toast.error((error as Error).message)
      setSigningOut(false)
    }
  }
  if (isAuthMessage) {
    return (
      <div className='auth-state'>
        <span className='brand-icon'>
          <img src='/logo.webp' alt='Lazy Campus' />
        </span>
        <h1>
          {t(pathname === '/signed-out' ? 'Signed out' : 'Access denied')}
        </h1>
        <p>
          {t(
            pathname === '/signed-out'
              ? 'Please reopen the platform when you need it.'
              : 'This platform is available to school users and the administrator.'
          )}
        </p>
        <a href='/'>
          {t('Return to platform')}
          <ArrowUpRight size={14} />
        </a>
      </div>
    )
  }
  if (
    !isHome &&
    (session.isPending ||
      (session.error instanceof ApiError && session.error.status === 401))
  ) {
    return (
      <div className='auth-state' role='status'>
        <span className='brand-icon'>
          <img src='/logo.webp' alt='Lazy Campus' />
        </span>
        <p>{t('Signing you in')}</p>
        <span className='loading-line' />
      </div>
    )
  }
  if (!isHome && session.error) {
    return (
      <div className='auth-state'>
        <ErrorState
          error={session.error}
          retry={() => void session.refetch()}
        />
      </div>
    )
  }
  if (!isHome && !session.data) return <Loading />
  const user = session.data?.user
  let page: ReactNode = <Overview />
  if (isHome) page = <Home />
  if (pathname === '/status') page = <ServiceStatusPage />
  if (pathname === '/analytics') page = <UsageAnalytics />
  if (pathname === '/notifications') page = <Notifications />
  if (pathname === '/sessions') page = <Devices />
  if (pathname === '/admin/overview') page = <UsageAnalytics admin />
  if (pathname === '/admin/apps') page = <AdminApplications />
  if (pathname === '/admin/endpoints') page = <AdminEndpoints />
  if (pathname === '/admin/settings') page = <AdminSettings />
  if (pathname === '/apps') page = <Applications />
  if (pathname === '/api-reference') page = <ApiReference />
  if (pathname === '/logs') page = <Logs />
  if (pathname === '/account' || pathname === '/profile') page = <Profile />
  if (pathname === '/admin/users') page = <AdminUsers />
  if (pathname === '/admin/logs') page = <Logs admin />
  if (pathname === '/admin/audits') page = <AuditLogs />
  if (pathname === '/admin/announcements') page = <AdminAnnouncements />
  if (pathname === '/admin/system') page = <AdminSystem />
  if (pathname.startsWith('/admin/') && user?.role !== 'admin') {
    page = <ErrorState error={new Error(t('Access denied'))} />
  }
  return (
    <SessionContext.Provider value={session.data || null}>
      <PreferencesProvider
        key={user?.id || 0}
        userId={user?.id || 0}
        dark={dark}
        changeTheme={setDark}
      >
        <PlatformLayout
          user={user}
          pathname={pathname}
          onSignOut={() => setLogoutOpen(true)}
        >
          {page}
        </PlatformLayout>
        <ConfirmDialog
          open={logoutOpen}
          onOpenChange={setLogoutOpen}
          title={t('Sign out')}
          desc={t('Please reopen the platform when you need it.')}
          confirmText={t('Sign out')}
          isLoading={signingOut}
          handleConfirm={() => void signOut()}
        />
      </PreferencesProvider>
    </SessionContext.Provider>
  )
}

function PlatformLayout(props: {
  user?: User
  pathname: string
  onSignOut: () => void
  children: ReactNode
}) {
  const { t, i18n } = useTranslation()
  const preferences = usePreferences()
  const [menu, setMenu] = useState(false)
  const [collapsed, setCollapsed] = useState(
    () => readPreference('platform-sidebar-collapsed') === 'true'
  )
  const [scrolled, setScrolled] = useState(false)
  const isHome = props.pathname === '/' || props.pathname === '/status'
  const site = useSite()
  const isAdmin = props.user?.role === 'admin'
  useEffect(() => {
    writePreference('platform-sidebar-collapsed', String(collapsed))
  }, [collapsed])
  useEffect(() => {
    if (!isHome) return
    const update = () => setScrolled(window.scrollY > 32)
    update()
    window.addEventListener('scroll', update, { passive: true })
    return () => window.removeEventListener('scroll', update)
  }, [isHome])
  return (
    <div
      className={`${isHome ? 'home-shell' : 'platform-shell'} ${collapsed ? 'sidebar-collapsed' : ''}`}
    >
      <a
        className='skip-content'
        href='#content'
        onClick={(event) => {
          if (props.pathname !== '/api-reference') return
          const content = document.querySelector<HTMLElement>('#content')
          if (content) {
            event.preventDefault()
            content.focus({ preventScroll: true })
            content.scrollIntoView({ block: 'start' })
          }
        }}
      >
        {t('Skip to content')}
      </a>
      <header
        className={`site-header ${isHome && scrolled ? 'is-scrolled' : ''}`}
      >
        <div className='site-header-left'>
          {!isHome && (
            <Button
              variant='ghost'
              size='icon'
              className='desktop-sidebar-toggle'
              aria-label={t('Toggle sidebar')}
              aria-expanded={!collapsed}
              aria-controls='platform-sidebar'
              onClick={() => setCollapsed(!collapsed)}
            >
              <PanelLeftClose size={18} />
            </Button>
          )}
          {!isHome && (
            <Button
              variant='ghost'
              size='icon'
              className='mobile-menu'
              aria-label={t(menu ? 'Close menu' : 'Open menu')}
              aria-expanded={menu}
              aria-controls='platform-sidebar'
              onClick={() => setMenu(!menu)}
            >
              {menu ? <X size={19} /> : <Menu size={19} />}
            </Button>
          )}
          <Link to='/' className='site-brand'>
            <span className='brand-icon'>
              <img src={site.data?.site.logo_url || '/logo.webp'} alt='' />
            </span>
            <strong>{site.data?.site.name || 'Lazy Campus'}</strong>
          </Link>
        </div>
        <div className='site-header-right'>
          <nav className='site-navigation' aria-label={t('Main navigation')}>
            <Link to='/' className={isHome ? 'selected' : ''}>
              {t('Home')}
            </Link>
            <Link to='/dashboard' className={!isHome ? 'selected' : ''}>
              {t('Console')}
            </Link>
            <Link to='/api-reference'>{t('API reference')}</Link>
          </nav>
          <AnnouncementBell key={props.user?.id || 0} userId={props.user?.id} />
          {props.user && <PersonalBell />}
          <Button
            variant='ghost'
            className='language-button'
            aria-label={t('Interface language')}
            onClick={() =>
              void i18n.changeLanguage(
                i18n.language.startsWith('zh') ? 'en' : 'zh'
              )
            }
          >
            {i18n.language.startsWith('zh') ? 'EN' : '中'}
          </Button>
          <Button
            variant='ghost'
            size='icon'
            aria-label={t('Switch theme')}
            onClick={() => preferences.changeTheme(!preferences.dark)}
          >
            {preferences.dark ? <Sun size={18} /> : <Moon size={18} />}
          </Button>
          {props.user && (
            <details
              className='avatar-menu'
              onKeyDown={(event) => {
                if (event.key === 'Escape') {
                  event.currentTarget.open = false
                  event.currentTarget.querySelector('summary')?.focus()
                }
              }}
              onBlur={(event) => {
                if (
                  !event.currentTarget.contains(event.relatedTarget as Node)
                ) {
                  event.currentTarget.open = false
                }
              }}
            >
              <summary aria-label={t('Profile menu')}>
                <span className='user-avatar'>
                  {props.user.username.slice(0, 1).toUpperCase()}
                </span>
              </summary>
              <div>
                <strong>{props.user.username}</strong>
                <small>{t(isAdmin ? 'Administrator' : 'School account')}</small>
                <Link to='/profile'>
                  <UserRound size={16} />
                  {t('Profile')}
                </Link>
                <button type='button' onClick={props.onSignOut}>
                  <LogOut size={16} />
                  {t('Sign out')}
                </button>
              </div>
            </details>
          )}
        </div>
      </header>
      {isHome ? (
        props.children
      ) : (
        <>
          {menu && (
            <button
              type='button'
              className='sidebar-scrim'
              aria-label={t('Close menu')}
              onClick={() => setMenu(false)}
            />
          )}
          <aside
            id='platform-sidebar'
            className={`platform-sidebar ${menu ? 'is-open' : ''}`}
            onKeyDown={(event) => {
              if (event.key === 'Escape') {
                setMenu(false)
                document
                  .querySelector<HTMLButtonElement>('.mobile-menu')
                  ?.focus()
              }
            }}
          >
            <div className='nav-caption'>{t('General')}</div>
            <nav aria-label={t('Workspace')}>
              {navigation
                .filter((item) => preferences.sidebar[item.to] !== false)
                .map((item) => (
                  <Link
                    key={item.to}
                    to={item.to}
                    title={t(item.label)}
                    aria-label={t(item.label)}
                    onClick={() => setMenu(false)}
                    className={
                      props.pathname === item.to
                        ? 'nav-item active'
                        : 'nav-item'
                    }
                  >
                    <item.icon size={17} />
                    <span className='nav-label'>{t(item.label)}</span>
                  </Link>
                ))}
            </nav>
            <div className='nav-caption'>{t('Personal')}</div>
            <nav aria-label={t('Personal')}>
              {[
                {
                  to: '/notifications',
                  label: 'Notifications',
                  icon: Activity,
                },
                { to: '/sessions', label: 'Login devices', icon: Monitor },
              ].map((item) => (
                <Link
                  key={item.to}
                  to={item.to}
                  title={t(item.label)}
                  aria-label={t(item.label)}
                  onClick={() => setMenu(false)}
                  className={
                    props.pathname === item.to ? 'nav-item active' : 'nav-item'
                  }
                >
                  <item.icon size={17} />
                  <span className='nav-label'>{t(item.label)}</span>
                </Link>
              ))}
              <Link
                to='/profile'
                title={t('Profile')}
                aria-label={t('Profile')}
                onClick={() => setMenu(false)}
                className={
                  ['/profile', '/account'].includes(props.pathname)
                    ? 'nav-item active'
                    : 'nav-item'
                }
              >
                <UserRound size={17} />
                <span className='nav-label'>{t('Profile')}</span>
              </Link>
            </nav>
            {isAdmin && (
              <>
                <div className='nav-caption'>{t('Administration')}</div>
                <nav aria-label={t('Administration')}>
                  {administration.map((item) => (
                    <Link
                      key={item.to}
                      to={item.to}
                      title={t(item.label)}
                      aria-label={t(item.label)}
                      onClick={() => setMenu(false)}
                      className={
                        props.pathname === item.to
                          ? 'nav-item active'
                          : 'nav-item'
                      }
                    >
                      <item.icon size={17} />
                      <span className='nav-label'>{t(item.label)}</span>
                    </Link>
                  ))}
                </nav>
              </>
            )}
            <div className='sidebar-bottom'>
              <div className='free-status'>
                <span />
                {t('Free access')}
              </div>
              <a
                href='https://github.com/Gradient-Clipping/lazycampus-platform'
                target='_blank'
                rel='noreferrer'
              >
                {t('View source')}
                <ArrowUpRight size={14} />
              </a>
            </div>
          </aside>
          <div className='platform-body'>
            <main className='platform-main' id='content' tabIndex={-1}>
              {props.children}
            </main>
            <footer className='platform-footer'>
              <span>
                {site.data?.site.name || 'Lazy Campus'} · {t('Open platform')}
              </span>
              <a
                href='https://github.com/QuantumNous/new-api'
                target='_blank'
                rel='noreferrer'
              >
                Powered by new-api
              </a>
            </footer>
          </div>
        </>
      )}
    </div>
  )
}
