import {
  keepPreviousData,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { toast } from 'sonner'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { Dialog } from '@/components/dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { translate, useLocale } from '@/i18n/translate'

import { formatDate, request } from './api'
import { statusLabels } from './operations-data'
import { Field, RefreshButton } from './operations-shared'
import { scopeName, scopeNames } from './permissions'
import { ErrorState, Loading, PageHeading, Pager } from './shared'
import type {
  Endpoint,
  Page,
  PlatformPolicy,
  SiteContent,
  Token,
  User,
} from './types'

export function AdminApplications() {
  useLocale()
  const cache = useQueryClient()
  const [page, setPage] = useState(1),
    [search, setSearch] = useState(''),
    [filter, setFilter] = useState(''),
    [status, setStatus] = useState('')
  const [editing, setEditing] = useState<Token | null>(null),
    [action, setAction] = useState('suspend'),
    [reason, setReason] = useState(''),
    [pending, setPending] = useState(false),
    [error, setError] = useState('')
  const apps = useQuery({
    queryKey: ['admin-tokens', page, filter, status],
    queryFn: () =>
      request<Page<Token> & { users: User[] }>(
        `/admin/tokens?page=${page}&search=${encodeURIComponent(filter)}&status=${status}`
      ),
    placeholderData: keepPreviousData,
  })
  async function save(e: FormEvent) {
    e.preventDefault()
    if (!editing) return
    setPending(true)
    setError('')
    try {
      await request(`/admin/tokens/${editing.id}`, 'PATCH', { action, reason })
      setEditing(null)
      await Promise.all([
        cache.invalidateQueries({ queryKey: ['admin-tokens'] }),
        cache.invalidateQueries({ queryKey: ['tokens'] }),
      ])
      toast.success(translate('应用授权已更新'))
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setPending(false)
    }
  }
  function edit(token: Token) {
    setEditing(token)
    setAction(token.suspended ? 'resume' : 'suspend')
    setReason('')
    setError('')
  }
  return (
    <>
      <PageHeading
        title={translate('应用监管')}
        action={
          <RefreshButton
            pending={apps.isFetching}
            onClick={() => void apps.refetch()}
          />
        }
      />
      <form
        className='list-toolbar'
        onSubmit={(e) => {
          e.preventDefault()
          setFilter(search)
          setPage(1)
        }}
      >
        <Input
          aria-label={translate('搜索用户或应用')}
          placeholder={translate('搜索用户或应用')}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <select
          aria-label={translate('应用状态')}
          value={status}
          onChange={(e) => {
            setStatus(e.target.value)
            setPage(1)
          }}
        >
          <option value=''>{translate('全部状态')}</option>
          <option value='active'>{translate('使用中')}</option>
          <option value='suspended'>{translate('管理员暂停')}</option>
          <option value='revoked'>{translate('已撤销授权')}</option>
          <option value='expired'>{translate('已到期')}</option>
          <option value='disabled'>{translate('用户停用')}</option>
        </select>
        <Button type='submit' variant='outline'>
          {translate('搜索')}
        </Button>
      </form>
      {apps.isPending && <Loading />}
      {apps.error && <ErrorState error={apps.error} />}
      {apps.data && (
        <Card className='table-card'>
          <StaticDataTable
            data={apps.data.items}
            getRowKey={(row) => row.id}
            emptyContent={translate('没有符合条件的应用')}
            columns={[
              {
                id: 'name',
                header: translate('应用'),
                cell: (t) => (
                  <div>
                    <strong>{t.name}</strong>
                    <small className='ops-block'>
                      #{t.id} · {t.key_prefix}••••
                    </small>
                  </div>
                ),
              },
              {
                id: 'user',
                header: translate('所属用户'),
                cell: (t) => {
                  const u = apps.data.users.find((u) => u.id === t.user_id)
                  return (
                    <span>
                      {u?.username || '—'}
                      <small className='ops-block'>#{t.user_id}</small>
                    </span>
                  )
                },
              },
              {
                id: 'status',
                header: translate('状态'),
                cell: (t) => (
                  <Badge
                    variant={
                      t.revoked_at || t.suspended ? 'destructive' : 'secondary'
                    }
                  >
                    {applicationStatus(t)}
                  </Badge>
                ),
              },
              {
                id: 'quota',
                header: translate('额度 / 限流'),
                cell: (t) => (
                  <div>
                    {t.daily_quota.toLocaleString()}
                    {translate(' / 天')}
                    <small className='ops-block'>
                      {t.rate_limit}
                      {translate(' / 分钟')}
                    </small>
                  </div>
                ),
              },
              {
                id: 'total',
                header: translate('总额度用量'),
                cell: (t) =>
                  `${t.used_quota.toLocaleString()} / ${t.quota === 0 ? translate('无限') : t.quota.toLocaleString()}`,
              },
              {
                id: 'expiry',
                header: translate('到期时间'),
                cell: (t) =>
                  t.expires_at ? formatDate(t.expires_at) : translate('长期'),
              },
              {
                id: 'action',
                header: translate('管理'),
                cell: (t) => (
                  <Button size='sm' variant='outline' onClick={() => edit(t)}>
                    {translate('查看 / 管理')}
                  </Button>
                ),
              },
            ]}
          />
          <Pager page={page} total={apps.data.total} onChange={setPage} />
        </Card>
      )}
      <Dialog
        open={editing !== null}
        onOpenChange={(open) => {
          if (!open && !pending) setEditing(null)
        }}
        title={
          editing
            ? translate('应用：{{v0}}', { v0: editing.name })
            : translate('应用监管')
        }
      >
        {editing && (
          <form className='ops-stack' onSubmit={save}>
            <dl className='ops-detail'>
              <div>
                <dt>{translate('状态')}</dt>
                <dd>{applicationStatus(editing)}</dd>
              </div>
              <div>
                <dt>{translate('权限')}</dt>
                <dd>{scopeNames(editing.scopes)}</dd>
              </div>
              <div>
                <dt>{translate('IP 白名单')}</dt>
                <dd>{editing.allowed_ips || translate('未限制')}</dd>
              </div>
              <div>
                <dt>{translate('最近调用')}</dt>
                <dd>
                  {editing.last_used_at
                    ? formatDate(editing.last_used_at)
                    : translate('尚未调用')}
                </dd>
              </div>
              {editing.admin_reason && (
                <div>
                  <dt>{translate('监管原因')}</dt>
                  <dd>{editing.admin_reason}</dd>
                </div>
              )}
            </dl>
            <a
              className='ops-text-link'
              href={`/admin/logs?token_id=${editing.id}`}
            >
              {translate('查看调用日志')}
            </a>
            {!editing.revoked_at && (
              <fieldset disabled={pending} className='ops-stack'>
                <Field label={translate('操作')}>
                  <select
                    value={action}
                    onChange={(e) => setAction(e.target.value)}
                  >
                    <option value='suspend'>{translate('暂停应用')}</option>
                    <option value='resume'>{translate('恢复应用')}</option>
                    <option value='revoke'>{translate('永久撤销授权')}</option>
                  </select>
                </Field>
                <Field label={translate('原因')}>
                  <Textarea
                    required
                    maxLength={160}
                    value={reason}
                    onChange={(e) => setReason(e.target.value)}
                  />
                </Field>
                {action === 'revoke' && (
                  <p role='note' className='ops-warning'>
                    {translate(
                      '撤销后，此应用的密钥、编辑和轮换授权将失效，无法恢复。'
                    )}
                  </p>
                )}
                {error && (
                  <p role='alert' className='form-error'>
                    {error}
                  </p>
                )}
                <Button
                  type='submit'
                  variant={action === 'revoke' ? 'destructive' : 'default'}
                >
                  {pending ? translate('保存中…') : translate('确认操作')}
                </Button>
              </fieldset>
            )}
          </form>
        )}
      </Dialog>
    </>
  )
}
function applicationStatus(t: Token) {
  if (t.revoked_at) return translate('已撤销授权')
  if (t.suspended) return translate('管理员暂停')
  if (!t.enabled) return translate('用户停用')
  if (t.expires_at && new Date(t.expires_at) <= new Date()) {
    return translate('已到期')
  }
  return translate('使用中')
}

const scopes = [
  'timetable:read',
  'grades:read',
  'exams:read',
  'calendar:read',
  'rooms:read',
  'messages:read',
  'notices:read',
  'electricity:read',
  'content:read',
]
export function AdminEndpoints() {
  useLocale()
  const cache = useQueryClient(),
    [editing, setEditing] = useState<Endpoint | null>(null),
    [pending, setPending] = useState(false),
    [error, setError] = useState('')
  const endpoints = useQuery({
    queryKey: ['admin-endpoints'],
    queryFn: () => request<Endpoint[]>('/admin/endpoints'),
  })
  async function save(e: FormEvent) {
    e.preventDefault()
    setPending(true)
    setError('')
    try {
      await request('/admin/endpoints', 'PUT', editing)
      setEditing(null)
      await Promise.all([
        cache.invalidateQueries({ queryKey: ['admin-endpoints'] }),
        cache.invalidateQueries({ queryKey: ['catalog'] }),
        cache.invalidateQueries({ queryKey: ['site'] }),
      ])
      toast.success(translate('接口设置已生效'))
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setPending(false)
    }
  }
  return (
    <>
      <PageHeading
        title={translate('接口管理')}
        action={
          <RefreshButton
            pending={endpoints.isFetching}
            onClick={() => void endpoints.refetch()}
          />
        }
      />
      {endpoints.isPending && <Loading />}
      {endpoints.error && <ErrorState error={endpoints.error} />}
      <div className='ops-endpoints'>
        {endpoints.data?.map((e) => (
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
            <code>GET /v1{e.path}</code>
            <p>{translate(e.description)}</p>
            <div className='ops-endpoint-footer'>
              <span>
                {e.rate_limit}
                {translate(' / 分钟 · ')}
                {e.daily_quota.toLocaleString()}
                {translate(' / 天')}
                <br />
                <small title={e.scope}>{scopeName(e.scope)}</small>
              </span>
              <Button
                size='sm'
                variant='outline'
                onClick={() => {
                  setEditing({ ...e })
                  setError('')
                }}
              >
                {translate('设置')}
              </Button>
            </div>
            {e.message && <p className='ops-warning'>{translate(e.message)}</p>}
          </Card>
        ))}
      </div>
      <Dialog
        open={editing !== null}
        onOpenChange={(open) => {
          if (!open && !pending) setEditing(null)
        }}
        title={translate('接口设置')}
      >
        {editing && (
          <form className='ops-stack' onSubmit={save}>
            <code>/v1{editing.path}</code>
            <fieldset className='ops-form-grid' disabled={pending}>
              <Field label={translate('名称')}>
                <Input
                  required
                  maxLength={80}
                  value={editing.name}
                  onChange={(e) =>
                    setEditing({ ...editing, name: e.target.value })
                  }
                />
              </Field>
              <Field label={translate('状态')}>
                <select
                  value={editing.status}
                  onChange={(e) =>
                    setEditing({
                      ...editing,
                      status: e.target.value as Endpoint['status'],
                    })
                  }
                >
                  {Object.entries(statusLabels).map(([key, label]) => (
                    <option key={key} value={key}>
                      {translate(label)}
                    </option>
                  ))}
                </select>
              </Field>
              <Field label={translate('所需权限')}>
                <select
                  value={editing.scope}
                  onChange={(e) =>
                    setEditing({ ...editing, scope: e.target.value })
                  }
                >
                  {[
                    ...new Set([
                      ...scopes,
                      ...(endpoints.data?.map((e) => e.scope) || []),
                    ]),
                  ].map((scope) => (
                    <option key={scope} value={scope}>
                      {scopeName(scope)} · {scope}
                    </option>
                  ))}
                </select>
              </Field>
              <Field label={translate('每用户每分钟上限')}>
                <Input
                  type='number'
                  required
                  min={1}
                  max={300}
                  value={editing.rate_limit}
                  onChange={(e) =>
                    setEditing({
                      ...editing,
                      rate_limit: e.target.valueAsNumber,
                    })
                  }
                />
              </Field>
              <Field label={translate('每用户每天上限')}>
                <Input
                  type='number'
                  required
                  min={1}
                  max={50000}
                  value={editing.daily_quota}
                  onChange={(e) =>
                    setEditing({
                      ...editing,
                      daily_quota: e.target.valueAsNumber,
                    })
                  }
                />
              </Field>
              <Field label={translate('同一用户调用间隔（秒）')}>
                <Input
                  type='number'
                  required
                  min={0}
                  max={3600}
                  value={editing.cooldown_seconds}
                  onChange={(e) =>
                    setEditing({
                      ...editing,
                      cooldown_seconds: e.target.valueAsNumber,
                    })
                  }
                />
              </Field>
              <Field label={translate('状态说明')} wide>
                <Input
                  maxLength={160}
                  value={editing.message}
                  onChange={(e) =>
                    setEditing({ ...editing, message: e.target.value })
                  }
                />
              </Field>
              <Field label={translate('接口文档说明')} wide>
                <Textarea
                  required
                  maxLength={4000}
                  rows={5}
                  value={editing.description}
                  onChange={(e) =>
                    setEditing({ ...editing, description: e.target.value })
                  }
                />
              </Field>
            </fieldset>
            {error && (
              <p role='alert' className='form-error'>
                {error}
              </p>
            )}
            <Button disabled={pending} type='submit'>
              {pending ? translate('保存中…') : translate('保存并生效')}
            </Button>
          </form>
        )}
      </Dialog>
    </>
  )
}

type Setting<T> = { value: T; updated_at: string }
export function AdminSettings() {
  useLocale()
  const [tab, setTab] = useState('policy')
  const policy = useQuery({
    queryKey: ['settings-policy'],
    queryFn: () => request<Setting<PlatformPolicy>>('/admin/settings/policy'),
  })
  const site = useQuery({
    queryKey: ['settings-site'],
    queryFn: () => request<Setting<SiteContent>>('/admin/settings/site'),
  })
  return (
    <>
      <PageHeading title={translate('站点与额度设置')} />
      <div className='segmented-tabs ops-tab-row'>
        <button
          type='button'
          aria-pressed={tab === 'policy'}
          onClick={() => setTab('policy')}
        >
          {translate('额度与限流')}
        </button>
        <button
          type='button'
          aria-pressed={tab === 'site'}
          onClick={() => setTab('site')}
        >
          {translate('品牌与内容')}
        </button>
      </div>
      {tab === 'policy' && policy.data && (
        <PolicyForm key={policy.data.updated_at} initial={policy.data} />
      )}
      {tab === 'policy' && policy.error && <ErrorState error={policy.error} />}
      {tab === 'policy' && policy.isPending && <Loading />}
      {tab === 'site' && site.data && (
        <SiteForm key={site.data.updated_at} initial={site.data} />
      )}
      {tab === 'site' && site.error && <ErrorState error={site.error} />}
      {tab === 'site' && site.isPending && <Loading />}
    </>
  )
}
function useSaveSettings(kind: 'policy' | 'site') {
  const cache = useQueryClient(),
    [pending, setPending] = useState(false),
    [error, setError] = useState('')
  async function save(value: unknown, updated_at: string) {
    setPending(true)
    setError('')
    try {
      await request(`/admin/settings/${kind}`, 'PUT', { value, updated_at })
      await Promise.all(
        [`settings-${kind}`, 'site', 'session', 'users', 'system'].map((key) =>
          cache.invalidateQueries({ queryKey: [key] })
        )
      )
      toast.success(translate('设置已生效'))
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setPending(false)
    }
  }
  return { save, pending, error }
}
function PolicyForm({ initial }: { initial: Setting<PlatformPolicy> }) {
  useLocale()
  const [value, setValue] = useState(initial.value),
    { save, pending, error } = useSaveSettings('policy')
  const fields: [keyof PlatformPolicy, string, number, number, string?][] = [
    [
      'daily_quota',
      translate('默认账户每日额度'),
      1,
      50000,
      translate('跟随默认的账户立即生效；自定义账户保持原值。'),
    ],
    ['rate_limit', translate('默认账户每分钟上限'), 1, 300],
    ['app_quota', translate('新应用总额度'), 1, 10000000],
    ['app_days', translate('新应用有效天数'), 1, 365],
    ['max_apps', translate('每账户应用数量'), 1, 50],
    ['burst', translate('每账户每秒突发上限'), 1, 30],
    ['user_concurrency', translate('每账户同时请求数'), 1, 10],
    ['global_concurrency', translate('全站同时请求数'), 1, 100],
  ]
  return (
    <Card className='ops-settings-card'>
      <form
        className='ops-stack'
        onSubmit={(e) => {
          e.preventDefault()
          void save(value, initial.updated_at)
        }}
      >
        <h2>{translate('默认额度与保护限制')}</h2>
        <fieldset disabled={pending} className='ops-form-grid'>
          {fields.map(([key, label, min, max, hint]) => (
            <Field key={key} label={label} hint={hint}>
              <Input
                type='number'
                required
                min={min}
                max={max}
                value={value[key]}
                onChange={(e) =>
                  setValue({ ...value, [key]: e.target.valueAsNumber })
                }
              />
            </Field>
          ))}
        </fieldset>
        <p className='quiet-text'>
          {translate(
            '应用总额度和有效期仅用于新建应用。修改限制不会清空已有用量。接口还会执行各自的限制。'
          )}
        </p>
        {error && (
          <p role='alert' className='form-error'>
            {error}
          </p>
        )}
        <div>
          <Button type='submit' disabled={pending}>
            {pending ? translate('保存中…') : translate('保存并生效')}
          </Button>
        </div>
      </form>
    </Card>
  )
}
function SiteForm({ initial }: { initial: Setting<SiteContent> }) {
  useLocale()
  const [value, setValue] = useState(initial.value),
    { save, pending, error } = useSaveSettings('site')
  const change = (key: keyof SiteContent, v: unknown) =>
    setValue({ ...value, [key]: v })
  return (
    <Card className='ops-settings-card'>
      <form
        className='ops-stack'
        onSubmit={(e) => {
          e.preventDefault()
          void save(value, initial.updated_at)
        }}
      >
        <h2>{translate('品牌与帮助信息')}</h2>
        <fieldset disabled={pending} className='ops-form-grid'>
          <Field label={translate('站点名称')}>
            <Input
              required
              maxLength={60}
              value={value.name}
              onChange={(e) => change('name', e.target.value)}
            />
          </Field>
          <Field
            label={translate('Logo 路径')}
            hint={translate('使用本站资源路径，例如 /logo.webp')}
          >
            <Input
              required
              value={value.logo_url}
              onChange={(e) => change('logo_url', e.target.value)}
            />
          </Field>
          <Field label={translate('站点简介')} wide>
            <Input
              maxLength={300}
              value={value.description}
              onChange={(e) => change('description', e.target.value)}
            />
          </Field>
          <Field label={translate('帮助页面')}>
            <Input
              placeholder={translate('https://… 或 /api-reference')}
              value={value.help_url}
              onChange={(e) => change('help_url', e.target.value)}
            />
          </Field>
          <Field label={translate('联系邮箱')}>
            <Input
              type='email'
              value={value.support_email}
              onChange={(e) => change('support_email', e.target.value)}
            />
          </Field>
          <Field label={translate('服务状态')}>
            <select
              value={value.service_status}
              onChange={(e) => change('service_status', e.target.value)}
            >
              {Object.entries(statusLabels)
                .filter(([k]) => k !== 'disabled')
                .map(([k, v]) => (
                  <option key={k} value={k}>
                    {translate(v)}
                  </option>
                ))}
            </select>
          </Field>
          <Field label={translate('服务状态说明')}>
            <Input
              maxLength={300}
              value={value.service_message}
              onChange={(e) => change('service_message', e.target.value)}
            />
          </Field>
          <div className='ops-field-wide ops-stack'>
            <div className='ops-card-heading'>
              <h2>{translate('常见问题')}</h2>
              <Button
                type='button'
                variant='outline'
                disabled={value.faq.length >= 30}
                onClick={() =>
                  change('faq', [
                    ...value.faq,
                    { id: crypto.randomUUID(), question: '', answer: '' },
                  ])
                }
              >
                {translate('添加问题')}
              </Button>
            </div>
            {value.faq.map((faq, index) => (
              <div className='ops-faq-edit' key={faq.id || faq.question}>
                <Field label={translate('问题 {{v0}}', { v0: index + 1 })}>
                  <Input
                    required
                    maxLength={200}
                    value={faq.question}
                    onChange={(e) =>
                      change(
                        'faq',
                        value.faq.map((f, i) =>
                          i === index ? { ...f, question: e.target.value } : f
                        )
                      )
                    }
                  />
                </Field>
                <Field label={translate('回答')}>
                  <Textarea
                    required
                    maxLength={3000}
                    value={faq.answer}
                    onChange={(e) =>
                      change(
                        'faq',
                        value.faq.map((f, i) =>
                          i === index ? { ...f, answer: e.target.value } : f
                        )
                      )
                    }
                  />
                </Field>
                <Button
                  type='button'
                  variant='ghost'
                  onClick={() =>
                    change(
                      'faq',
                      value.faq.filter((_, i) => i !== index)
                    )
                  }
                >
                  {translate('删除问题')}
                </Button>
              </div>
            ))}
          </div>
        </fieldset>
        {error && (
          <p role='alert' className='form-error'>
            {error}
          </p>
        )}
        <div>
          <Button type='submit' disabled={pending}>
            {pending ? translate('保存中…') : translate('保存并生效')}
          </Button>
        </div>
      </form>
    </Card>
  )
}
