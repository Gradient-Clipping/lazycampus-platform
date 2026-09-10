import { Link } from '@tanstack/react-router'

import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { translate, useLocale, dateLocale } from '@/i18n/translate'

import { useSite, statusLabels } from './operations-data'
import { RefreshButton } from './operations-shared'
import { ErrorState, Loading } from './shared'

export function SiteHelp() {
  useLocale()
  const site = useSite()
  if (!site.data) return null
  const s = site.data.site
  return (
    <section className='home-notices ops-site-help'>
      <div className='home-section-heading'>
        <span>HELP & STATUS</span>
        <h2>{translate('帮助与服务状态')}</h2>
      </div>
      <div className='ops-help-status'>
        <Link to='/status'>
          <span className={`ops-status-dot ${s.service_status}`} />
          {translate(statusLabels[s.service_status])}
          <span>{translate('查看接口状态 →')}</span>
        </Link>
        {s.help_url && <a href={s.help_url}>{translate('帮助文档 ↗')}</a>}
        {s.support_email && (
          <a href={`mailto:${s.support_email}`}>{s.support_email}</a>
        )}
      </div>
      {s.service_message && (
        <p className='ops-service-message'>{translate(s.service_message)}</p>
      )}
      {s.faq.length > 0 && (
        <div className='ops-faq-list'>
          {s.faq.map((faq) => (
            <details key={faq.id || faq.question}>
              <summary>{translate(faq.question)}</summary>
              <p>{translate(faq.answer)}</p>
            </details>
          ))}
        </div>
      )}
    </section>
  )
}
export function ServiceStatusPage() {
  useLocale()
  const data = useSite()
  return (
    <main id='content' className='home-main ops-status-page'>
      <div className='home-section-heading'>
        <span>SERVICE STATUS</span>
        <h1>{translate('服务状态')}</h1>
      </div>
      <div className='ops-actions'>
        <RefreshButton
          pending={data.isFetching}
          onClick={() => void data.refetch()}
        />
      </div>
      {data.isPending && <Loading />}
      {data.error && <ErrorState error={data.error} />}
      {data.data && (
        <div className='ops-stack'>
          <Card className='ops-status-overall'>
            <span
              className={`ops-status-dot ${data.data.site.service_status}`}
            />
            <h2>{translate(statusLabels[data.data.site.service_status])}</h2>
            {data.data.site.service_message && (
              <p>{translate(data.data.site.service_message)}</p>
            )}
          </Card>
          <div className='ops-endpoints'>
            {data.data.endpoints.map((e) => (
              <Card key={e.path} className='ops-endpoint'>
                <div className='ops-card-heading'>
                  <h2>{translate(e.name)}</h2>
                  <Badge
                    variant={
                      e.status === 'operational' ? 'secondary' : 'destructive'
                    }
                  >
                    {translate(statusLabels[e.status])}
                  </Badge>
                </div>
                <code>/v1{e.path}</code>
                {e.message && <p>{translate(e.message)}</p>}
              </Card>
            ))}
          </div>
          <small className='quiet-text'>
            {translate('状态由平台维护。最近更新于')}{' '}
            {new Date(data.dataUpdatedAt).toLocaleTimeString(dateLocale())}
          </small>
        </div>
      )}
      <Link className='ops-text-link' to='/'>
        {translate('返回首页')}
      </Link>
    </main>
  )
}
