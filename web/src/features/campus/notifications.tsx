import {
  keepPreviousData,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { BellRing, Mail } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { translate, useLocale, currentLanguage } from '@/i18n/translate'

import { formatLoginDate, request } from './api'
import { Field, RefreshButton } from './operations-shared'
import { ErrorState, Loading, PageHeading, Pager } from './shared'
import type {
  NotificationPreference,
  Page,
  PersonalNotification,
} from './types'

export function PersonalBell() {
  useLocale()
  const unread = useQuery({
    queryKey: ['notification-unread'],
    queryFn: () => request<{ count: number }>('/notifications/unread'),
    refetchInterval: 60000,
  })
  return (
    <Link
      to='/notifications'
      className='ops-notification-bell'
      aria-label={
        unread.data?.count
          ? translate('个人通知，{{count}} 条未读', {
              count: unread.data.count,
            })
          : translate('个人通知')
      }
      title={translate('个人通知')}
    >
      <BellRing size={18} />
      {!!unread.data?.count && <span>{Math.min(99, unread.data.count)}</span>}
    </Link>
  )
}
const deliveryLabels: Record<string, string> = {
  sent: '邮件已提交',
  queued: '等待发送邮件',
  sending: '正在提交邮件',
  retry: '邮件等待重试',
  failed: '邮件发送失败',
  uncertain: '邮件发送结果待确认',
  throttled: '邮件达到发送上限',
  disabled: '邮件未开启',
}
export function Notifications() {
  useLocale()
  const cache = useQueryClient(),
    [page, setPage] = useState(1),
    [unread, setUnread] = useState(false),
    [kind, setKind] = useState(''),
    [pending, setPending] = useState(false)
  const notices = useQuery({
    queryKey: ['notifications', page, unread, kind],
    queryFn: () =>
      request<Page<PersonalNotification>>(
        `/notifications?page=${page}&unread=${unread}&kind=${kind}`
      ),
    placeholderData: keepPreviousData,
  })
  async function mark(id: number | 'all') {
    setPending(true)
    try {
      await request(`/notifications/${id}/read`, 'POST')
      await Promise.all([
        cache.invalidateQueries({ queryKey: ['notifications'] }),
        cache.invalidateQueries({ queryKey: ['notification-unread'] }),
      ])
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setPending(false)
    }
  }
  return (
    <>
      <PageHeading
        title={translate('个人通知')}
        action={
          <div className='ops-actions'>
            <Button
              variant='outline'
              disabled={pending}
              onClick={() => void mark('all')}
            >
              {translate('全部已读')}
            </Button>
            <RefreshButton
              pending={notices.isFetching}
              onClick={() => void notices.refetch()}
            />
          </div>
        }
      />
      <div className='list-toolbar'>
        <label className='checkbox-label'>
          <input
            type='checkbox'
            checked={unread}
            onChange={(e) => {
              setUnread(e.target.checked)
              setPage(1)
            }}
          />
          {translate('只看未读')}
        </label>
        <select
          aria-label={translate('通知类型')}
          value={kind}
          onChange={(e) => {
            setKind(e.target.value)
            setPage(1)
          }}
        >
          <option value=''>{translate('全部类型')}</option>
          {Object.entries({
            quota: translate('额度'),
            errors: translate('错误率'),
            rate: translate('限流'),
            expiry: translate('应用到期'),
            application: translate('应用授权'),
          }).map(([k, v]) => (
            <option key={k} value={k}>
              {v}
            </option>
          ))}
        </select>
        <Link to='/profile' hash='notifications' className='ops-text-link'>
          {translate('通知偏好')}
        </Link>
      </div>
      {notices.isPending && <Loading />}
      {notices.error && <ErrorState error={notices.error} />}
      {notices.data && (
        <div className='ops-stack'>
          {notices.data.items.length === 0 && (
            <Card className='empty-state'>{translate('暂无通知')}</Card>
          )}
          {notices.data.items.map((n) => (
            <Card
              key={n.id}
              className={`ops-notification ${n.in_app && !n.read_at ? 'is-unread' : ''}`}
            >
              <div className='ops-card-heading'>
                <h2>{translate(n.title)}</h2>
                {n.in_app && !n.read_at && <Badge>{translate('未读')}</Badge>}
              </div>
              <p>{translate(n.content)}</p>
              <div className='ops-notification-footer'>
                <small>
                  {formatLoginDate(n.created_at)} ·{' '}
                  {n.in_app ? translate('站内通知') : translate('邮件通知')}
                  {n.email_status !== 'disabled' &&
                    ` · ${translate(deliveryLabels[n.email_status] || n.email_status)}`}
                </small>
                {!n.read_at && (
                  <Button
                    variant='ghost'
                    size='sm'
                    disabled={pending}
                    onClick={() => void mark(n.id)}
                  >
                    {translate('标为已读')}
                  </Button>
                )}
              </div>
              {['errors', 'rate'].includes(n.kind) && (
                <Link to='/logs' className='ops-text-link'>
                  {translate('查看调用日志')}
                </Link>
              )}
            </Card>
          ))}
          <Pager page={page} total={notices.data.total} onChange={setPage} />
        </div>
      )}
    </>
  )
}
interface PreferencesResponse {
  preferences: NotificationPreference
  email_verified: boolean
  email_available: boolean
}
export function NotificationPreferences() {
  useLocale()
  const data = useQuery({
    queryKey: ['notification-preferences'],
    queryFn: () => request<PreferencesResponse>('/notification-preferences'),
  })
  if (data.isPending) return <Loading />
  if (data.error) return <ErrorState error={data.error} />
  return (
    <PreferenceForm
      key={data.data.preferences.updated_at}
      initial={data.data}
    />
  )
}
function PreferenceForm({ initial }: { initial: PreferencesResponse }) {
  useLocale()
  const cache = useQueryClient(),
    [form, setForm] = useState({
      ...initial.preferences,
      language: initial.preferences.language || currentLanguage(),
    }),
    [pending, setPending] = useState(false),
    [error, setError] = useState(''),
    [code, setCode] = useState(''),
    [codeSent, setCodeSent] = useState(false)
  const verified =
    initial.email_verified && form.email === initial.preferences.email
  const emailAllowed = verified && initial.email_available && !!form.email
  const [saving, setSaving] = useState(false)
  async function save(e: FormEvent) {
    e.preventDefault()
    setSaving(true)
    setError('')
    try {
      await request('/notification-preferences', 'PUT', form)
      await cache.invalidateQueries({ queryKey: ['notification-preferences'] })
      toast.success(translate('通知偏好已保存'))
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setSaving(false)
    }
  }
  async function sendCode() {
    setPending(true)
    setError('')
    try {
      if (form.email !== initial.preferences.email) {
        await request('/notification-preferences', 'PUT', {
          ...form,
          email_enabled: false,
        })
      }
      await request('/notification-preferences/send-verification', 'POST')
      setCodeSent(true)
      toast.success(translate('验证码已发送'))
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setPending(false)
    }
  }
  async function verify() {
    setPending(true)
    setError('')
    try {
      await request('/notification-preferences/verify', 'POST', { code })
      await cache.invalidateQueries({ queryKey: ['notification-preferences'] })
      toast.success(translate('通知邮箱已验证'))
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setPending(false)
    }
  }
  function toggle(key: keyof NotificationPreference, value: boolean) {
    setForm({ ...form, [key]: value })
  }
  return (
    <form className='ops-stack ops-notification-preferences' onSubmit={save}>
      <fieldset disabled={saving || pending} className='ops-stack'>
        <Field label={translate('通知语言')}>
          <select
            value={form.language}
            onChange={(event) =>
              setForm({ ...form, language: event.target.value as 'zh' | 'en' })
            }
          >
            <option value='zh'>简体中文</option>
            <option value='en'>English</option>
          </select>
        </Field>
        <Field label={translate('通知邮箱')}>
          <Input
            type='email'
            maxLength={254}
            placeholder={translate('用于接收个人告警')}
            value={form.email}
            onChange={(e) => {
              setForm({ ...form, email: e.target.value, email_enabled: false })
              setCodeSent(false)
            }}
          />
        </Field>
        <div className='ops-actions'>
          {verified ? (
            <Badge variant='secondary'>{translate('邮箱已验证')}</Badge>
          ) : (
            <Button
              variant='outline'
              type='button'
              disabled={!form.email || !initial.email_available}
              onClick={() => void sendCode()}
            >
              <Mail size={16} />
              {codeSent ? translate('重新发送验证码') : translate('验证邮箱')}
            </Button>
          )}
          {!initial.email_available && (
            <span className='quiet-text'>{translate('邮件服务暂不可用')}</span>
          )}
        </div>
        {codeSent && !verified && (
          <div className='ops-code-verification'>
            <Input
              aria-label={translate('邮箱验证码')}
              inputMode='numeric'
              autoComplete='one-time-code'
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value)}
              placeholder={translate('6 位验证码')}
            />
            <Button
              type='button'
              disabled={code.length !== 6}
              onClick={() => void verify()}
            >
              {translate('确认验证码')}
            </Button>
          </div>
        )}
        <div className='ops-form-grid'>
          <label className='checkbox-label'>
            <input
              type='checkbox'
              checked={form.in_app}
              onChange={(e) => toggle('in_app', e.target.checked)}
            />
            {translate('站内通知')}
          </label>
          <label className='checkbox-label'>
            <input
              type='checkbox'
              checked={form.email_enabled}
              disabled={!emailAllowed}
              onChange={(e) => toggle('email_enabled', e.target.checked)}
            />
            {translate('邮件通知')}
          </label>
        </div>
        {!emailAllowed && (
          <p className='quiet-text'>
            {translate('设置并验证邮箱后可开启邮件通知。')}
          </p>
        )}
        <div className='ops-alert-preference'>
          <label className='checkbox-label'>
            <input
              type='checkbox'
              checked={form.quota_enabled}
              onChange={(e) => toggle('quota_enabled', e.target.checked)}
            />
            {translate('额度提醒')}
          </label>
          <Field label={translate('每日或应用总额度达到（%）')}>
            <Input
              type='number'
              required
              min={10}
              max={100}
              disabled={!form.quota_enabled}
              value={form.quota_percent}
              onChange={(e) =>
                setForm({ ...form, quota_percent: e.target.valueAsNumber })
              }
            />
          </Field>
        </div>
        <div className='ops-alert-preference'>
          <label className='checkbox-label'>
            <input
              type='checkbox'
              checked={form.errors_enabled}
              onChange={(e) => toggle('errors_enabled', e.target.checked)}
            />
            {translate('错误率提醒')}
          </label>
          <div className='ops-form-grid'>
            <Field label={translate('15 分钟错误率达到（%）')}>
              <Input
                type='number'
                required
                min={1}
                max={100}
                disabled={!form.errors_enabled}
                value={form.error_percent}
                onChange={(e) =>
                  setForm({ ...form, error_percent: e.target.valueAsNumber })
                }
              />
            </Field>
            <Field label={translate('至少有多少次请求')}>
              <Input
                type='number'
                required
                min={5}
                max={1000}
                disabled={!form.errors_enabled}
                value={form.error_minimum}
                onChange={(e) =>
                  setForm({ ...form, error_minimum: e.target.valueAsNumber })
                }
              />
            </Field>
          </div>
        </div>
        <div className='ops-alert-preference'>
          <label className='checkbox-label'>
            <input
              type='checkbox'
              checked={form.rate_enabled}
              onChange={(e) => toggle('rate_enabled', e.target.checked)}
            />
            {translate('触发额度或限流时提醒')}
          </label>
        </div>
        <div className='ops-alert-preference'>
          <label className='checkbox-label'>
            <input
              type='checkbox'
              checked={form.expiry_enabled}
              onChange={(e) => toggle('expiry_enabled', e.target.checked)}
            />
            {translate('应用到期提醒')}
          </label>
          <Field label={translate('提前天数')}>
            <Input
              type='number'
              required
              min={1}
              max={30}
              disabled={!form.expiry_enabled}
              value={form.expiry_days}
              onChange={(e) =>
                setForm({ ...form, expiry_days: e.target.valueAsNumber })
              }
            />
          </Field>
        </div>
      </fieldset>
      {error && (
        <p role='alert' className='form-error'>
          {error}
        </p>
      )}
      <div>
        <Button type='submit' disabled={saving || pending}>
          {saving ? translate('保存中…') : translate('保存通知偏好')}
        </Button>
      </div>
    </form>
  )
}
