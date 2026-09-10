import { useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, Plus, RotateCcw, Settings2, Trash2 } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { CopyButton } from '@/components/copy-button'
import { Dialog } from '@/components/dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { translate } from '@/i18n/translate'

import { formatDate, request } from './api'
import { scopeName } from './permissions'
import { useSession } from './session'
import { ErrorState, Loading, PageHeading } from './shared'
import type { Endpoint, PlatformPolicy, Token, TokenInput, User } from './types'

export function Applications() {
  const { t } = useTranslation()
  const { user, limits } = useSession()
  const cache = useQueryClient()
  const tokens = useQuery({
    queryKey: ['tokens'],
    queryFn: () =>
      request<{ items: Token[]; daily_usage: Record<string, number> }>(
        '/tokens'
      ),
  })
  const catalog = useQuery({
    queryKey: ['catalog'],
    queryFn: () => request<Endpoint[]>('/catalog'),
  })
  const [editing, setEditing] = useState<Token | 'new' | null>(null)
  const [secret, setSecret] = useState('')
  const [confirm, setConfirm] = useState<{
    token: Token
    action: 'delete' | 'rotate'
  } | null>(null)
  const [pending, setPending] = useState(false)
  const refresh = () => {
    void cache.invalidateQueries({ queryKey: ['tokens'] })
    void cache.invalidateQueries({ queryKey: ['dashboard'] })
  }
  async function performAction() {
    if (!confirm) return
    setPending(true)
    try {
      if (confirm.action === 'rotate') {
        const result = await request<{ key: string }>(
          `/tokens/${confirm.token.id}/rotate`,
          'POST'
        )
        setSecret(result.key)
      } else await request(`/tokens/${confirm.token.id}`, 'DELETE')
      setConfirm(null)
      refresh()
    } catch (error) {
      toast.error((error as Error).message)
    } finally {
      setPending(false)
    }
  }
  return (
    <>
      <PageHeading
        title={t('Applications')}
        description={t('The account quota is shared by all applications.')}
        action={
          <Button
            onClick={() => setEditing('new')}
            disabled={
              !catalog.data ||
              (tokens.data?.items.length || 0) >= (limits?.max_apps || 10)
            }
          >
            <Plus size={16} />
            {t('Create application')}
          </Button>
        }
      />
      {tokens.isPending && <Loading />}
      {tokens.error && (
        <ErrorState error={tokens.error} retry={() => void tokens.refetch()} />
      )}
      {catalog.error && (
        <ErrorState
          error={catalog.error}
          retry={() => void catalog.refetch()}
        />
      )}
      {tokens.data?.items.length === 0 && (
        <Card>
          <div className='empty-state'>
            <div className='empty-icon'>
              <KeyRound />
            </div>
            <h2>{t('No applications yet')}</h2>
            <p>{t('Create a key to connect your project.')}</p>
            <Button onClick={() => setEditing('new')} disabled={!catalog.data}>
              {t('Create application')}
            </Button>
          </div>
        </Card>
      )}
      <div className='applications-grid'>
        {tokens.data?.items.map((token) => {
          let status = 'Enabled'
          if (token.revoked_at) status = 'Authorization revoked'
          else if (token.suspended) status = 'Suspended by admin'
          else if (!token.enabled) status = 'Disabled'
          else if (
            token.expires_at &&
            new Date(token.expires_at) <= new Date()
          ) {
            status = 'Expired'
          }
          const used = tokens.data.daily_usage[token.id] || 0
          return (
            <Card key={token.id} className='application-card'>
              <CardContent>
                <div className='application-title'>
                  <span className='application-icon'>
                    <BoxIcon />
                  </span>
                  <div>
                    <h2>{token.name}</h2>
                    <code>{token.key_prefix}••••••••</code>
                  </div>
                  <Badge
                    variant={status === 'Enabled' ? 'secondary' : 'outline'}
                  >
                    {t(status)}
                  </Badge>
                </div>
                <div className='quota-label'>
                  <span>{t('Today')}</span>
                  <strong>
                    {used.toLocaleString()}{' '}
                    <span>/ {token.daily_quota.toLocaleString()}</span>
                  </strong>
                </div>
                <meter
                  min={0}
                  max={token.daily_quota}
                  value={used}
                  aria-label={t('Daily quota')}
                />
                <div className='application-limits'>
                  <span>{token.rate_limit} / min</span>
                  <span>
                    {t('Remaining')}{' '}
                    {token.quota === 0
                      ? translate('无限')
                      : Math.max(
                          0,
                          token.quota - token.used_quota
                        ).toLocaleString()}
                  </span>
                </div>
                <div className='scope-badges'>
                  {token.scopes.split(' ').map((scope) => (
                    <span key={scope} title={scope}>
                      {scopeName(scope)}
                    </span>
                  ))}
                </div>
                <div className='application-footer'>
                  <span>
                    {token.expires_at
                      ? `${t('Expires')} ${formatDate(token.expires_at)}`
                      : t('Long term')}
                  </span>
                  <div>
                    <Button
                      variant='ghost'
                      size='icon'
                      aria-label={`${t('Edit')} ${token.name}`}
                      onClick={() => setEditing(token)}
                      disabled={!!token.revoked_at}
                    >
                      <Settings2 size={15} />
                    </Button>
                    <Button
                      variant='ghost'
                      size='icon'
                      aria-label={`${t('Rotate key')} ${token.name}`}
                      onClick={() => setConfirm({ token, action: 'rotate' })}
                      disabled={!!token.revoked_at || token.suspended}
                    >
                      <RotateCcw size={15} />
                    </Button>
                    <Button
                      variant='ghost'
                      size='icon'
                      aria-label={`${t('Delete')} ${token.name}`}
                      onClick={() => setConfirm({ token, action: 'delete' })}
                    >
                      <Trash2 size={15} />
                    </Button>
                  </div>
                </div>
              </CardContent>
              {token.admin_reason && (
                <p className='ops-app-reason'>{token.admin_reason}</p>
              )}
            </Card>
          )
        })}
      </div>
      {editing && catalog.data && (
        <ApplicationEditor
          key={editing === 'new' ? 'new' : editing.id}
          token={editing === 'new' ? undefined : editing}
          user={user}
          limits={limits}
          catalog={catalog.data}
          onClose={() => setEditing(null)}
          onSaved={(key) => {
            setEditing(null)
            if (key) setSecret(key)
            refresh()
          }}
        />
      )}
      <Dialog
        open={!!secret}
        onOpenChange={(open) => {
          if (!open) setSecret('')
        }}
        title={t('Your application key')}
        description={t('Copy it now. You cannot view it again.')}
        footer={<Button onClick={() => setSecret('')}>{t('Done')}</Button>}
      >
        <div className='secret-box'>
          <code>{secret}</code>
          <CopyButton value={secret} />
        </div>
        <p className='quiet-text'>
          {t('Keep keys out of public repositories and browser code.')}
        </p>
      </Dialog>
      <ConfirmDialog
        open={!!confirm}
        onOpenChange={(open) => {
          if (!open) setConfirm(null)
        }}
        title={t(
          confirm?.action === 'rotate'
            ? 'Rotate this key?'
            : 'Delete application?'
        )}
        desc={t(
          confirm?.action === 'rotate'
            ? 'The current key will stop working immediately.'
            : 'Its key will stop working immediately. Request logs are retained.'
        )}
        destructive
        isLoading={pending}
        handleConfirm={() => void performAction()}
      />
    </>
  )
}
function BoxIcon() {
  return <KeyRound size={19} />
}

function ApplicationEditor(props: {
  token?: Token
  user: User
  limits?: PlatformPolicy
  catalog: Endpoint[]
  onClose: () => void
  onSaved: (key?: string) => void
}) {
  const { t } = useTranslation()
  const token = props.token
  const [unlimitedQuota, setUnlimitedQuota] = useState(token?.quota === 0)
  const allScopes = [...new Set(props.catalog.map((e) => e.scope))]
  const [form, setForm] = useState<TokenInput>({
    name: token?.name || '',
    scopes: token?.scopes.split(' ') || ['timetable:read', 'calendar:read'],
    allowed_ips: token?.allowed_ips || '',
    daily_quota: token?.daily_quota || props.user.daily_quota,
    rate_limit: token?.rate_limit || props.user.rate_limit,
    quota: token?.quota || props.limits?.app_quota || 100000,
    never_expires: token?.expires_at === null,
    expires_in_days: token?.expires_at
      ? Math.max(
          1,
          Math.ceil((Date.parse(token.expires_at) - Date.now()) / 86400000)
        )
      : props.limits?.app_days || 90,
    enabled: token?.enabled ?? true,
  })
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  async function save(event: FormEvent) {
    event.preventDefault()
    setError('')
    if (!form.scopes.length) {
      setError(t('No permissions selected'))
      return
    }
    if (
      form.daily_quota > props.user.daily_quota ||
      form.rate_limit > props.user.rate_limit
    ) {
      setError(t('Quota cannot exceed account limits'))
      return
    }
    setPending(true)
    try {
      const result = await request<{ key?: string }>(
        token ? `/tokens/${token.id}` : '/tokens',
        token ? 'PATCH' : 'POST',
        { ...form, quota: unlimitedQuota ? 0 : form.quota }
      )
      props.onSaved(result.key)
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setPending(false)
    }
  }
  const numberFields: {
    key: 'daily_quota' | 'rate_limit'
    label: string
    max: number
  }[] = [
    { key: 'daily_quota', label: 'Daily quota', max: props.user.daily_quota },
    {
      key: 'rate_limit',
      label: 'Requests per minute',
      max: props.user.rate_limit,
    },
  ]
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) props.onClose()
      }}
      title={t(token ? 'Edit application' : 'Create application')}
      description={`${props.user.daily_quota.toLocaleString()} / ${t('Daily quota')}`}
      footer={
        <>
          <Button variant='outline' disabled={pending} onClick={props.onClose}>
            {t('Cancel')}
          </Button>
          <Button form='application-form' type='submit' disabled={pending}>
            {t(pending ? 'Saving' : 'Save')}
          </Button>
        </>
      }
    >
      <form
        id='application-form'
        className='editor-form'
        onSubmit={(event) => void save(event)}
      >
        <label>
          {t('Application name')}
          <Input
            autoFocus
            required
            maxLength={60}
            value={form.name}
            placeholder={t('My campus script')}
            onChange={(event) => setForm({ ...form, name: event.target.value })}
          />
        </label>
        <fieldset>
          <legend>{t('API permissions')}</legend>
          <div className='scope-picker'>
            {allScopes.map((scope) => (
              <label key={scope}>
                <input
                  type='checkbox'
                  aria-label={scopeName(scope)}
                  aria-description={scope}
                  checked={form.scopes.includes(scope)}
                  onChange={(event) =>
                    setForm({
                      ...form,
                      scopes: event.target.checked
                        ? [...form.scopes, scope]
                        : form.scopes.filter((item) => item !== scope),
                    })
                  }
                />
                <span className='scope-label'>
                  <span>{scopeName(scope)}</span>
                  <small>{scope}</small>
                </span>
              </label>
            ))}
          </div>
          <Button
            variant='ghost'
            type='button'
            onClick={() =>
              setForm({
                ...form,
                scopes:
                  form.scopes.length === allScopes.length ? [] : allScopes,
              })
            }
          >
            {t(
              form.scopes.length === allScopes.length
                ? 'Clear selection'
                : 'Select all'
            )}
          </Button>
        </fieldset>
        <div className='form-grid'>
          {numberFields.map((field) => (
            <label key={field.key}>
              {t(field.label)}
              <Input
                type='number'
                min={1}
                max={field.max}
                required
                value={form[field.key]}
                onChange={(event) =>
                  setForm({ ...form, [field.key]: Number(event.target.value) })
                }
              />
            </label>
          ))}
          <div className='validity-field'>
            <label htmlFor='application-total-quota'>{t('Total quota')}</label>
            <div className='validity-row'>
              <Input
                id='application-total-quota'
                type='number'
                min={Math.max(1, token?.used_quota || 1)}
                max={10000000}
                required={!unlimitedQuota}
                disabled={unlimitedQuota}
                value={form.quota}
                onChange={(event) =>
                  setForm({ ...form, quota: Number(event.target.value) })
                }
              />
              <label className='checkbox-label'>
                <input
                  type='checkbox'
                  checked={unlimitedQuota}
                  onChange={(event) => setUnlimitedQuota(event.target.checked)}
                />
                {translate('无限')}
              </label>
            </div>
          </div>
          <div className='validity-field'>
            <label htmlFor='application-validity'>
              {t('Validity in days')}
            </label>
            <div className='validity-row'>
              <Input
                id='application-validity'
                type='number'
                min={1}
                max={365}
                required={!form.never_expires}
                disabled={form.never_expires}
                value={form.expires_in_days}
                onChange={(event) =>
                  setForm({
                    ...form,
                    expires_in_days: Number(event.target.value),
                  })
                }
              />
              <label className='checkbox-label'>
                <input
                  type='checkbox'
                  checked={form.never_expires}
                  onChange={(event) =>
                    setForm({ ...form, never_expires: event.target.checked })
                  }
                />
                {t('Long term')}
              </label>
            </div>
          </div>
        </div>
        <label>
          {t('IP allowlist')}
          <Input
            value={form.allowed_ips}
            maxLength={1000}
            onChange={(event) =>
              setForm({ ...form, allowed_ips: event.target.value })
            }
          />
          <small>
            {t('Leave empty for any IP. Separate IPs or CIDRs with spaces.')}
          </small>
        </label>
        {token && (
          <label className='checkbox-label'>
            <input
              type='checkbox'
              checked={form.enabled}
              onChange={(event) =>
                setForm({ ...form, enabled: event.target.checked })
              }
            />
            {t('Enabled')}
          </label>
        )}
        {error && (
          <p role='alert' className='form-error'>
            {error}
          </p>
        )}
      </form>
    </Dialog>
  )
}
