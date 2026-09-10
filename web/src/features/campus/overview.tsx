import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import {
  ArrowRight,
  ArrowUpRight,
  Box,
  CheckCircle2,
  Gauge,
  Terminal,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'

import { formatDate, request } from './api'
import { CodeBlock } from './code-block'
import { useSession } from './session'
import { ErrorState, Loading, PageHeading } from './shared'
import type { Dashboard } from './types'

export function Overview() {
  const { t } = useTranslation()
  const user = useSession().user
  const stats = useQuery({
    queryKey: ['dashboard'],
    queryFn: () => request<Dashboard>('/dashboard'),
  })
  const notice = useQuery({
    queryKey: ['notice'],
    queryFn: () => request<{ content: string }>('/notice'),
  })
  if (stats.isPending) return <Loading />
  if (stats.error) {
    return <ErrorState error={stats.error} retry={() => void stats.refetch()} />
  }
  const data = stats.data
  const successes = data.successful_24h + data.failed_24h
  const code =
    'curl "https://platform.lazycampus.com/v1/teaching/timetable" \\\n  -H "Authorization: Bearer $LAZYCAMPUS_API_KEY"'
  const max = Math.max(1, ...data.series.map((item) => item.requests))
  const days = Array.from({ length: 14 }, (_, index) => {
    const date = new Date()
    date.setUTCDate(date.getUTCDate() - 13 + index)
    const day = date.toISOString().slice(0, 10)
    return {
      day,
      requests: data.series.find((item) => item.day === day)?.requests || 0,
    }
  })
  const cards = [
    {
      label: 'Today',
      value: data.today_requests.toLocaleString(),
      detail: `${data.daily_quota.toLocaleString()} / ${t('Daily quota')}`,
      icon: Gauge,
    },
    {
      label: 'Available today',
      value: Math.max(
        0,
        data.daily_quota - data.today_requests
      ).toLocaleString(),
      detail: `${t('Reset at')} ${formatDate(data.reset_at)}`,
      icon: ArrowUpRight,
    },
    {
      label: 'Active applications',
      value: String(data.active_apps),
      detail: `${user.rate_limit} / ${t('Requests per minute')}`,
      icon: Box,
    },
    {
      label: 'Success rate',
      value: successes
        ? `${Math.round((data.successful_24h / successes) * 100)}%`
        : '—',
      detail: t('Last 24 hours'),
      icon: CheckCircle2,
    },
  ]
  return (
    <>
      <PageHeading
        title={t('Overview')}
        description={user.display_name || user.username}
        action={
          <Button render={<Link to='/apps' />}>
            {t('Create application')}
            <ArrowRight size={16} />
          </Button>
        }
      />
      <div className='stats-grid'>
        {cards.map((card) => (
          <Card key={card.label} className='stat-card'>
            <CardContent>
              <div className='stat-label'>
                {t(card.label)}
                <card.icon size={17} />
              </div>
              <strong className='stat-value'>{card.value}</strong>
              <span className='stat-detail'>{card.detail}</span>
            </CardContent>
          </Card>
        ))}
      </div>
      <Card className='chart-card'>
        <div className='section-heading'>
          <h2>{t('Request activity')}</h2>
          <span>{t('Last 14 days')}</span>
        </div>
        <div
          className='activity-chart'
          role='img'
          aria-label={`${t('Request activity')}: ${days.map((day) => `${day.day}: ${day.requests}`).join(', ')}`}
        >
          <div className='chart-axis'>
            <span>{max > 1 ? max.toLocaleString() : ''}</span>
            <span>0</span>
          </div>
          <div className='chart-bars'>
            {days.map((day, index) => (
              <div key={day.day} className='chart-day'>
                <div className='bar-track'>
                  <div
                    className='activity-bar'
                    style={{
                      height: `${Math.max(1, (day.requests / max) * 100)}%`,
                    }}
                    title={`${day.day} · ${day.requests} ${t('Requests')}`}
                    tabIndex={0}
                    aria-label={`${day.day}: ${day.requests}`}
                  />
                </div>
                <span>
                  {[0, 4, 8, 13].includes(index) ? day.day.slice(5) : ''}
                </span>
              </div>
            ))}
          </div>
        </div>
      </Card>
      <div className='overview-lower'>
        <Card className='quick-start'>
          <div className='section-heading'>
            <h2>
              <Terminal size={17} />
              {t('Quick start')}
            </h2>
            <Link to='/api-reference'>
              {t('Read API reference')}
              <ArrowUpRight size={14} />
            </Link>
          </div>
          <ol className='steps'>
            <li>
              <span>1</span>
              {t(
                'Create an application, choose permissions, and copy your key.'
              )}
            </li>
            <li>
              <span>2</span>
              {t('Use the key on your server or in your own script.')}
            </li>
          </ol>
          <div className='code-example'>
            <div>
              <span>cURL</span>
              <CopyButton value={code} />
            </div>
            <CodeBlock code={code} />
          </div>
          <p className='quiet-text'>
            {t('Keep keys out of public repositories and browser code.')}
          </p>
        </Card>
        <Card className='notice-card'>
          <div className='section-heading'>
            <h2>{t('Notice')}</h2>
            <span className='status-dot' />
          </div>
          <div className='notice-body'>
            {notice.data?.content || t('No notice')}
          </div>
          <div className='quota-note'>
            <span>{t('Free access')}</span>
            <p>{t('The account quota is shared by all applications.')}</p>
            <p>{t('Follow Retry-After when a request returns 429.')}</p>
          </div>
        </Card>
      </div>
    </>
  )
}
