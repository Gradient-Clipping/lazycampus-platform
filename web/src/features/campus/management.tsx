import {
  keepPreviousData,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { CheckCircle2, Settings2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { Dialog } from '@/components/dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { translate } from '@/i18n/translate'

import { formatLoginDate, formatDate, request } from './api'
import { ErrorState, Loading, PageHeading, Pager } from './shared'
import type { Audit, Page, System, User } from './types'

export function AdminUsers() {
  const { t } = useTranslation()
  const cache = useQueryClient()
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const [filter, setFilter] = useState('')
  const [editing, setEditing] = useState<User | null>(null)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const users = useQuery({
    queryKey: ['users', page, filter],
    queryFn: () =>
      request<Page<User>>(
        `/admin/users?page=${page}&search=${encodeURIComponent(filter)}`
      ),
    placeholderData: keepPreviousData,
  })
  async function save() {
    if (!editing) return
    setPending(true)
    setError('')
    try {
      await request(`/admin/users/${editing.id}`, 'PATCH', {
        enabled: editing.enabled,
        daily_quota: editing.daily_quota,
        rate_limit: editing.rate_limit,
        inherit_limits: editing.inherit_limits || false,
      })
      setEditing(null)
      void cache.invalidateQueries({ queryKey: ['users'] })
      void cache.invalidateQueries({ queryKey: ['session'] })
      toast.success(t('Saved'))
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setPending(false)
    }
  }
  return (
    <>
      <PageHeading title={t('Users')} />
      <form
        className='list-toolbar'
        onSubmit={(e) => {
          e.preventDefault()
          setFilter(search)
          setPage(1)
        }}
      >
        <Input
          aria-label={t('Search users')}
          placeholder={t('Search users')}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <Button type='submit' variant='outline'>
          {t('Search users')}
        </Button>
      </form>
      {users.isPending && <Loading />}
      {users.error && <ErrorState error={users.error} />}{' '}
      {users.data && (
        <Card className='table-card'>
          <StaticDataTable
            data={users.data.items}
            getRowKey={(u) => u.id}
            emptyContent={t('No users found')}
            columns={[
              {
                id: 'name',
                header: t('Username'),
                cell: (u) => (
                  <div>
                    <strong>{u.display_name || u.username}</strong>
                    <small className='quiet-text block'>{u.username}</small>
                  </div>
                ),
              },
              {
                id: 'role',
                header: t('Role'),
                cell: (u) =>
                  t(u.role === 'admin' ? 'Administrator' : 'School account'),
              },
              {
                id: 'status',
                header: t('Status'),
                cell: (u) => (
                  <Badge variant={u.enabled ? 'secondary' : 'destructive'}>
                    {t(u.enabled ? 'Enabled' : 'Disabled')}
                  </Badge>
                ),
              },
              {
                id: 'quota',
                header: t('Daily quota'),
                cell: (u) => u.daily_quota.toLocaleString(),
              },
              {
                id: 'rate',
                header: t('Requests per minute'),
                cell: (u) => u.rate_limit,
              },
              {
                id: 'login',
                header: t('Last login'),
                cell: (u) => formatLoginDate(u.last_login_at),
              },
              {
                id: 'actions',
                header: t('Actions'),
                cell: (u) => (
                  <Button
                    variant='ghost'
                    size='icon'
                    aria-label={`${t('Edit')} ${u.username}`}
                    onClick={() => {
                      setEditing(u)
                      setError('')
                    }}
                  >
                    <Settings2 size={16} />
                  </Button>
                ),
              },
            ]}
          />
          <Pager page={page} total={users.data.total} onChange={setPage} />
        </Card>
      )}
      <Dialog
        open={!!editing}
        onOpenChange={(open) => {
          if (!open && !pending) setEditing(null)
        }}
        title={t('Edit user')}
        footer={
          <Button form='user-editor' type='submit' disabled={pending}>
            {t(pending ? 'Saving' : 'Save')}
          </Button>
        }
      >
        {editing && (
          <form
            id='user-editor'
            className='editor-form'
            onSubmit={(event) => {
              event.preventDefault()
              void save()
            }}
          >
            <p>{editing.username}</p>
            <label className='checkbox-label'>
              <input
                type='checkbox'
                checked={editing.inherit_limits || false}
                onChange={(e) =>
                  setEditing({ ...editing, inherit_limits: e.target.checked })
                }
              />
              {translate('跟随全站默认额度')}
            </label>
            <label>
              {t('Daily quota')}
              <Input
                type='number'
                min={1}
                max={50000}
                required
                value={editing.daily_quota}
                disabled={editing.inherit_limits}
                onChange={(e) =>
                  setEditing({
                    ...editing,
                    daily_quota: Number(e.target.value),
                  })
                }
              />
            </label>
            <label>
              {t('Requests per minute')}
              <Input
                type='number'
                min={1}
                max={300}
                required
                value={editing.rate_limit}
                disabled={editing.inherit_limits}
                onChange={(e) =>
                  setEditing({ ...editing, rate_limit: Number(e.target.value) })
                }
              />
            </label>
            <label className='checkbox-label'>
              <input
                type='checkbox'
                checked={editing.enabled}
                onChange={(e) =>
                  setEditing({ ...editing, enabled: e.target.checked })
                }
              />
              {t('Enabled account')}
            </label>
            {error && (
              <p role='alert' className='form-error'>
                {error}
              </p>
            )}
          </form>
        )}
      </Dialog>
    </>
  )
}

export function AuditLogs() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [detail, setDetail] = useState<Audit | null>(null)
  const logs = useQuery({
    queryKey: ['audits', page],
    queryFn: () => request<Page<Audit>>(`/admin/audits?page=${page}`),
    placeholderData: keepPreviousData,
  })
  return (
    <>
      <PageHeading title={t('Audit logs')} />
      {logs.isPending && <Loading />}
      {logs.error && <ErrorState error={logs.error} />}{' '}
      {logs.data && (
        <Card className='table-card'>
          <StaticDataTable
            data={logs.data.items}
            getRowKey={(log) => log.id}
            emptyContent={t('No audit events')}
            columns={[
              {
                id: 'time',
                header: t('Time'),
                cell: (log) => formatDate(log.created_at),
              },
              {
                id: 'actor',
                header: t('Actor'),
                cell: (log) => `#${log.actor_id}`,
              },
              {
                id: 'action',
                header: t('Action'),
                cell: (log) => <code>{log.action}</code>,
              },
              { id: 'target', header: t('Target'), cell: (log) => log.target },
              {
                id: 'details',
                header: translate('变更详情'),
                cell: (log) =>
                  log.details ? (
                    <Button
                      variant='ghost'
                      size='sm'
                      onClick={() => setDetail(log)}
                    >
                      {translate('查看')}
                    </Button>
                  ) : (
                    '—'
                  ),
              },
            ]}
          />
          <Pager page={page} total={logs.data.total} onChange={setPage} />
        </Card>
      )}
      <Dialog
        open={!!detail}
        onOpenChange={(open) => {
          if (!open) setDetail(null)
        }}
        title={translate('变更详情')}
      >
        <pre className='ops-audit-details'>{detail?.details}</pre>
      </Dialog>
    </>
  )
}

export function AdminSystem() {
  const { t } = useTranslation()
  const system = useQuery({
    queryKey: ['system'],
    queryFn: () => request<System>('/admin/system'),
  })
  if (system.isPending) return <Loading />
  if (system.error) {
    return (
      <ErrorState error={system.error} retry={() => void system.refetch()} />
    )
  }
  const info = system.data
  return (
    <>
      <PageHeading
        title={t('System')}
        action={
          <Button variant='outline' onClick={() => void system.refetch()}>
            {t('Refresh')}
          </Button>
        }
      />
      <div className='system-health'>
        {[
          { name: 'Database', ok: info.database },
          { name: 'Rate limiter', ok: info.redis },
        ].map((item) => (
          <Card key={item.name}>
            <CheckCircle2 size={20} />
            <span>{t(item.name)}</span>
            <Badge variant={item.ok ? 'secondary' : 'destructive'}>
              {t(item.ok ? 'Healthy' : 'Unavailable')}
            </Badge>
          </Card>
        ))}
      </div>
      <Card className='system-details'>
        <dl className='details-grid'>
          {[
            { name: 'Running version', value: info.revision },
            { name: 'Total users', value: info.users },
            { name: 'Total applications', value: info.tokens },
            { name: 'Retained requests', value: info.requests },
            {
              name: translate('Sender 事务邮件'),
              value: info.email_available
                ? translate('已配置')
                : translate('未配置'),
            },
            {
              name: 'Concurrent requests per user',
              value: info.concurrency_per_user,
            },
            {
              name: 'Global concurrent requests',
              value: info.concurrency_global,
            },
            {
              name: 'Log retention',
              value: `${info.log_retention_days} ${t('days')}`,
            },
          ].map((item) => (
            <div key={item.name}>
              <dt>{t(item.name)}</dt>
              <dd>{item.value}</dd>
            </div>
          ))}
        </dl>
      </Card>
    </>
  )
}
