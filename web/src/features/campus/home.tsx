import { Link } from '@tanstack/react-router'
import {
  ArrowRight,
  ArrowUpRight,
  BookOpen,
  CalendarDays,
  Code2,
  Gauge,
  GraduationCap,
  KeyRound,
  Layers,
  ShieldCheck,
  Terminal,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Button } from '@/components/ui/button'
import { translate, currentLanguage } from '@/i18n/translate'

import { useAnnouncements } from './announcement-data'
import { formatDate } from './api'
import { CodeBlock } from './code-block'
import { useSite } from './operations-data'
import { scopeName } from './permissions'
import { SiteHelp } from './site-content'
import { StatusPageLink } from './status-page-link'

const demos = [
  {
    name: 'Timetable',
    path: '/teaching/timetable',
    scope: 'timetable:read',
    sample:
      '{\n  "success": true,\n  "data": {\n    "courses": [\n      { "courseName": "示例课程" }\n    ]\n  }\n}',
  },
  {
    name: 'Calendar',
    path: '/teaching/calendar',
    scope: 'calendar:read',
    sample:
      '{\n  "success": true,\n  "data": {\n    "academicYear": "2026-2027",\n    "startYear": 2026\n  }\n}',
  },
  {
    name: 'Exams',
    path: '/teaching/exams',
    scope: 'exams:read',
    sample: '{\n  "success": true,\n  "data": {\n    "items": []\n  }\n}',
  },
] as const

export function Home() {
  const { t } = useTranslation()
  const [demo, setDemo] = useState(0)
  const feed = useAnnouncements()
  const site = useSite()
  const dailyQuota = site.data?.limits.daily_quota
  const rateLimit = site.data?.limits.rate_limit
  const selected = demos[demo]
  const code = `curl "https://platform.lazycampus.com/v1${selected.path}" \\\n  -H "Authorization: Bearer $LAZYCAMPUS_API_KEY"`
  return (
    <main className='home-main' id='content'>
      <section className='home-hero'>
        <div className='home-hero-copy'>
          <span className='home-eyebrow'>
            <span />
            {t('Free campus APIs')}
          </span>
          <h1>
            {t('Connect your campus.')}
            <br />
            <span>{t('Build something yours.')}</span>
          </h1>
          <p>
            {t(
              'Bring your timetable, exams and campus services into your own apps with one simple API.'
            )}
          </p>
          <div className='home-hero-actions'>
            <Button render={<Link to='/dashboard' />}>
              {t('Go to dashboard')}
              <ArrowRight size={17} />
            </Button>
            <Button variant='outline' render={<Link to='/api-reference' />}>
              <BookOpen size={17} />
              {t('Documentation')}
            </Button>
          </div>
          <div className='home-integrations'>
            <p>{t('Built for your workflow')}</p>
            <div>
              <span>
                <Terminal size={17} />
                Python
              </span>
              <span>
                <Code2 size={17} />
                JavaScript
              </span>
              <span>
                <Layers size={17} />
                cURL
              </span>
            </div>
          </div>
        </div>
        <div className='home-api-demo'>
          <div
            className='demo-tabs'
            role='group'
            aria-label={t('API examples')}
          >
            {demos.map((item, index) => (
              <button
                type='button'
                key={item.name}
                aria-pressed={index === demo}
                onClick={() => setDemo(index)}
              >
                {t(item.name)}
              </button>
            ))}
            <span>{t('Example')}</span>
          </div>
          <div className='demo-endpoint'>
            <span className='method-badge'>GET</span>
            <code>/v1{selected.path}</code>
          </div>
          <div className='demo-code'>
            <div>
              <span>REQUEST</span>
              <CopyButton value={code} />
            </div>
            <CodeBlock key={selected.path} code={code} className='demo-fade' />
          </div>
          <div className='demo-response'>
            <span>{t('Response example')}</span>
            <CodeBlock
              key={selected.path}
              code={
                currentLanguage() === 'en'
                  ? selected.sample.replaceAll(
                      '示例课程',
                      translate('示例课程')
                    )
                  : selected.sample
              }
              language='json'
              className='demo-fade'
            />
          </div>
          <div className='demo-bottom'>
            <code>{scopeName(selected.scope)}</code>
            <span>{t('Free access')} · JSON</span>
          </div>
        </div>
      </section>
      <section className='home-numbers' aria-label={t('Platform limits')}>
        {[
          ['18', 'Campus endpoints'],
          ['9', 'Permission scopes'],
          [dailyQuota?.toLocaleString() || '—', 'Daily requests'],
          [rateLimit?.toLocaleString() || '—', 'Requests per minute'],
        ].map(([value, label]) => (
          <div key={label}>
            <strong>
              {value}
              <span>
                {label === 'Daily requests' && t('/ day')}
                {label === 'Requests per minute' && t('/ min')}
              </span>
            </strong>
            <p>{t(label)}</p>
          </div>
        ))}
      </section>
      <section className='home-features'>
        <div className='home-section-heading'>
          <span>{t('Core features')}</span>
          <h2>
            {t('For campus developers.')}
            <br />
            <span>{t('Ready for your next idea.')}</span>
          </h2>
        </div>
        <div className='home-feature-grid'>
          <article className='home-feature feature-data'>
            <span className='feature-number'>01</span>
            <CalendarDays size={27} />
            <h3>{t('Your campus, connected')}</h3>
            <p>
              {t(
                'Timetables, grades, exams, notices, classrooms and electricity in one place.'
              )}
            </p>
            <div className='feature-tags'>
              {[
                'Timetable',
                'Grades',
                'Exams',
                'Calendar',
                'Rooms',
                'Electricity',
              ].map((name) => (
                <span key={name}>{t(name)}</span>
              ))}
            </div>
          </article>
          <article className='home-feature'>
            <span className='feature-number'>02</span>
            <ShieldCheck size={27} />
            <h3>{t('Your data stays yours')}</h3>
            <p>
              {t(
                'School identity verification and scoped keys give each application only the access you choose.'
              )}
            </p>
            <div className='feature-visual-shield'>
              <ShieldCheck size={52} />
              <span>SSO</span>
              <span>API KEY</span>
            </div>
          </article>
          <article className='home-feature'>
            <span className='feature-number'>03</span>
            <Gauge size={27} />
            <h3>{t('Free, with fair limits')}</h3>
            <p>
              {dailyQuota?.toLocaleString() || '—'} /{' '}
              {t('/ day').replace('/', '')} · {rateLimit || '—'} / min
              {translate('。')}
              {t('The account quota is shared by all applications.')}
            </p>
            <div className='feature-quota'>
              <strong>
                {dailyQuota?.toLocaleString() || '—'}
                <small>{t('/ day')}</small>
              </strong>
              <div />
              <span>{t('No payment required')}</span>
            </div>
          </article>
          <article className='home-feature feature-open'>
            <span className='feature-number'>04</span>
            <Code2 size={27} />
            <h3>{t('Made for developers')}</h3>
            <p>
              {t(
                'Standard HTTP, clear documentation and request logs. Use the language you already know.'
              )}
            </p>
            <a
              href='https://github.com/Gradient-Clipping/lazycampus-platform'
              target='_blank'
              rel='noreferrer'
            >
              {t('View source')}
              <ArrowUpRight size={16} />
            </a>
          </article>
        </div>
      </section>
      <section className='home-workflow'>
        <div className='home-section-heading'>
          <span>{t('Workflow')}</span>
          <h2>{t('Get started in three steps')}</h2>
        </div>
        <div>
          {[
            {
              title: 'Enter the console',
              body: 'Continue with your school identity through single sign-on.',
              icon: GraduationCap,
            },
            {
              title: 'Create an application',
              body: 'Choose permissions and save your application key.',
              icon: KeyRound,
            },
            {
              title: 'Make your first request',
              body: 'Connect your app and track its requests in the console.',
              icon: ActivityIcon,
            },
          ].map((step, index) => (
            <article key={step.title}>
              <span>{index + 1}</span>
              <step.icon size={22} />
              <h3>{t(step.title)}</h3>
              <p>{t(step.body)}</p>
            </article>
          ))}
        </div>
      </section>
      <section className='home-notices'>
        <div className='home-section-heading'>
          <span>{t('Platform updates')}</span>
          <h2>{t('System announcements')}</h2>
        </div>
        {feed.data?.items.length ? (
          <div className='home-notice-grid'>
            {feed.data.items.slice(0, 3).map((item) => (
              <article key={item.id}>
                <time>{formatDate(item.created_at)}</time>
                <h3>{item.title}</h3>
                <p>{item.content}</p>
              </article>
            ))}
          </div>
        ) : (
          <p className='quiet-text'>
            {feed.error && t('Announcements unavailable')}
            {!feed.error && feed.isPending && t('Loading')}
            {!feed.error && !feed.isPending && t('No notice')}
          </p>
        )}
      </section>
      <SiteHelp />
      <footer className='home-footer'>
        <Link to='/' className='site-brand'>
          <span className='brand-icon'>
            <img src={site.data?.site.logo_url || '/logo.webp'} alt='' />
          </span>
          <strong>{site.data?.site.name || 'Lazy Campus'}</strong>
        </Link>
        <p>{translate(site.data?.site.description) || t('Free campus APIs')}</p>
        <div>
          <span>
            © {new Date().getFullYear()} {site.data?.site.name || 'Lazy Campus'}
          </span>
          <StatusPageLink />
          <a
            href='https://github.com/Gradient-Clipping/lazycampus-platform'
            target='_blank'
            rel='noreferrer'
          >
            {t('View source')}
          </a>
          <a
            href='https://github.com/QuantumNous/new-api'
            target='_blank'
            rel='noreferrer'
          >
            Powered by new-api
          </a>
        </div>
      </footer>
    </main>
  )
}
function ActivityIcon(props: { size: number }) {
  return <Gauge {...props} />
}
