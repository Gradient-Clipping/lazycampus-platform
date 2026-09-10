import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useRouterState } from '@tanstack/react-router'
import {
  Activity,
  ArrowUpRight,
  Gauge,
  KeyRound,
  Languages,
  LayoutGrid,
  Link2,
  Monitor,
  Settings2,
  ShieldCheck,
  Wallet,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { translate, useLocale } from '@/i18n/translate'

import { formatLoginDate, formatDate, request } from './api'
import { NotificationPreferences } from './notifications'
import { defaultSidebar, sidebarOptions, usePreferences } from './preferences'
import { useSession } from './session'
import type { Dashboard } from './types'

export function Profile() {
  const { t, i18n } = useTranslation()
  const { user } = useSession()
  const preferences = usePreferences()
  const navigate = useNavigate()
  const tab = useRouterState({
    select: (state) => {
      const hash = state.location.hash
      return hash === 'notifications' || hash === 'preferences'
        ? hash
        : 'identity'
    },
  })
  function setTab(hash: string) {
    void navigate({ hash, replace: true, resetScroll: false })
  }
  const [draft, setDraft] = useState(preferences.sidebar)
  const stats = useQuery({
    queryKey: ['dashboard'],
    queryFn: () => request<Dashboard>('/dashboard'),
  })
  const remaining = stats.data
    ? Math.max(0, user.daily_quota - stats.data.today_requests).toLocaleString()
    : '—'
  return (
    <div className='profile-page'>
      <Card className='profile-summary'>
        <div className='profile-summary-identity'>
          <span className='profile-avatar'>
            {user.username.slice(0, 1).toUpperCase()}
          </span>
          <div className='profile-summary-name'>
            <div>
              <h1>{user.username}</h1>
              <span>
                {t(user.role === 'admin' ? 'Administrator' : 'School account')}
              </span>
              <span className='profile-user-id'>
                {t('User ID')} {user.id}
              </span>
            </div>
            <p>
              @{user.username}
              <span>•</span>
              {user.display_name || user.username}
              <span>•</span>Lazy Campus
            </p>
          </div>
        </div>
        <div className='profile-metrics'>
          {[
            {
              label: 'Available today',
              value: remaining,
              detail: `${user.daily_quota.toLocaleString()} / ${t('Daily quota')}`,
              icon: Wallet,
              color: 'green',
            },
            {
              label: 'Today',
              value: stats.data?.today_requests.toLocaleString() ?? '—',
              detail: `${user.rate_limit} ${t('requests/min')}`,
              icon: Gauge,
              color: 'blue',
            },
            {
              label: 'API requests',
              value: stats.data?.total_requests.toLocaleString() ?? '—',
              detail: t('Last 60 days'),
              icon: Activity,
              color: 'purple',
            },
          ].map((item) => (
            <div key={item.label}>
              <span className='profile-metric-label'>
                <i className={item.color}>
                  <item.icon size={16} />
                </i>
                {t(item.label)}
              </span>
              <strong>{item.value}</strong>
              <small>{item.detail}</small>
            </div>
          ))}
        </div>
        {stats.error && (
          <div className='profile-stats-error' role='alert'>
            {t('Usage unavailable')}
            <Button variant='ghost' onClick={() => void stats.refetch()}>
              {t('Retry')}
            </Button>
          </div>
        )}
      </Card>
      <div className='profile-columns'>
        <div className='profile-primary'>
          <Card className='profile-settings'>
            <SectionTitle
              icon={Settings2}
              title={t('Settings')}
              detail={t('Account preferences and integrations')}
            />
            <div className='profile-settings-content'>
              <div
                className='segmented-tabs'
                role='group'
                aria-label={t('Profile settings')}
              >
                <button
                  type='button'
                  aria-pressed={tab === 'notifications'}
                  onClick={() => setTab('notifications')}
                >
                  <Settings2 size={16} />
                  {t('Notification preferences')}
                </button>
                <button
                  type='button'
                  aria-pressed={tab === 'identity'}
                  onClick={() => setTab('identity')}
                >
                  <Link2 size={16} />
                  {t('Account connections')}
                </button>
                <button
                  type='button'
                  aria-pressed={tab === 'preferences'}
                  onClick={() => setTab('preferences')}
                >
                  <Settings2 size={16} />
                  {t('Preferences')}
                </button>
              </div>
              {tab === 'identity' && (
                <div className='identity-connections'>
                  <div className='connection-item'>
                    <span className='connection-icon'>
                      <ShieldCheck size={22} />
                    </span>
                    <div>
                      <h3>
                        Keycloak{' '}
                        <span className='connection-status'>
                          {t('Connected')}
                        </span>
                      </h3>
                      <p>{t('Single sign-on')}</p>
                    </div>
                    <span className='connection-check'>
                      <ShieldCheck size={18} />
                    </span>
                  </div>
                  <div className='connection-item'>
                    <span className='connection-icon'>
                      <KeyRound size={21} />
                    </span>
                    <div>
                      <h3>{t('Identity')}</h3>
                      <p>
                        {t(
                          user.role === 'admin'
                            ? 'Administrator'
                            : 'School account'
                        )}
                      </p>
                    </div>
                    <span className='connection-status'>{t('Verified')}</span>
                  </div>
                </div>
              )}
              {tab === 'preferences' && (
                <div className='preference-fields'>
                  <label>
                    <span>
                      <Languages size={18} />
                      {t('Interface language')}
                    </span>
                    <select
                      aria-label={t('Interface language')}
                      value={i18n.language.startsWith('zh') ? 'zh' : 'en'}
                      onChange={(event) =>
                        void i18n.changeLanguage(event.target.value)
                      }
                    >
                      <option value='zh'>{translate('简体中文')}</option>
                      <option value='en'>English</option>
                    </select>
                  </label>
                  <label>
                    <span>
                      <Monitor size={18} />
                      {t('Appearance')}
                    </span>
                    <select
                      aria-label={t('Appearance')}
                      value={preferences.dark ? 'dark' : 'light'}
                      onChange={(event) =>
                        preferences.changeTheme(event.target.value === 'dark')
                      }
                    >
                      <option value='light'>{t('Light')}</option>
                      <option value='dark'>{t('Dark')}</option>
                    </select>
                  </label>
                </div>
              )}
              {tab === 'notifications' && <NotificationPreferences />}
            </div>
          </Card>
          <Card className='profile-settings'>
            <SectionTitle
              icon={ShieldCheck}
              title={t('Security')}
              detail={t('Identity and application access')}
            />
            <div className='security-items'>
              <div>
                <ShieldCheck size={21} />
                <span>
                  <h3>{t('Single sign-on')}</h3>
                  <p>{t('Identity is managed by the school and Keycloak.')}</p>
                </span>
                <span className='connection-status'>{t('Enabled')}</span>
              </div>
              <Link to='/apps'>
                <KeyRound size={21} />
                <span>
                  <h3>{t('API keys')}</h3>
                  <p>{t('Keys, permissions and quotas')}</p>
                </span>
                <ArrowUpRight size={17} />
              </Link>
              <Link to='/sessions'>
                <Monitor size={21} />
                <span>
                  <h3>{t('Login devices')}</h3>
                  <p>{translate('查看最近活动并退出设备')}</p>
                </span>
                <ArrowUpRight size={17} />
              </Link>
            </div>
          </Card>
          <Card className='profile-settings'>
            <SectionTitle
              icon={Monitor}
              title={t('Account activity')}
              detail={t('Your current platform account')}
            />
            <dl className='profile-activity'>
              <div>
                <dt>{t('Last sign-in')}</dt>
                <dd>{formatLoginDate(user.last_login_at)}</dd>
              </div>
              <div>
                <dt>{t('Created')}</dt>
                <dd>{formatDate(user.created_at)}</dd>
              </div>
              <div>
                <dt>{t('Authentication')}</dt>
                <dd>Keycloak SSO</dd>
              </div>
            </dl>
          </Card>
        </div>
        <Card className='profile-sidebar-settings'>
          <SectionTitle
            icon={LayoutGrid}
            title={t('Sidebar preferences')}
            detail={t('Choose what appears in your sidebar')}
          />
          <div className='sidebar-preferences-form'>
            {sidebarOptions.map((item) => (
              <label className='preference-switch-row' key={item.path}>
                <span>
                  <strong>{t(item.title)}</strong>
                  <small>{t(item.detail)}</small>
                </span>
                <input
                  type='checkbox'
                  role='switch'
                  aria-label={t(item.title)}
                  checked={draft[item.path] !== false}
                  onChange={(event) =>
                    setDraft({ ...draft, [item.path]: event.target.checked })
                  }
                />
                <span aria-hidden='true' className='preference-switch' />
              </label>
            ))}
            <p className='quiet-text'>
              {t(
                'Saved in this browser. Profile and administration stay accessible.'
              )}
            </p>
            <div className='sidebar-preferences-actions'>
              <Button
                variant='outline'
                onClick={() => setDraft(defaultSidebar)}
              >
                {t('Reset defaults')}
              </Button>
              <Button
                onClick={() => {
                  preferences.saveSidebar(draft)
                  toast.success(t('Saved'))
                }}
              >
                {t('Save changes')}
              </Button>
            </div>
          </div>
        </Card>
      </div>
    </div>
  )
}
function SectionTitle(props: {
  icon: typeof Settings2
  title: string
  detail: string
}) {
  useLocale()
  return (
    <div className='profile-section-title'>
      <span>
        <props.icon size={20} />
      </span>
      <div>
        <h2>{props.title}</h2>
        <p>{props.detail}</p>
      </div>
    </div>
  )
}
