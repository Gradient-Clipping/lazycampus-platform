import { useQuery } from '@tanstack/react-query'
import { useState, type FormEvent, type ReactNode } from 'react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { translate, useLocale } from '@/i18n/translate'

import { request } from './api'
import type { Endpoint } from './types'

export function Field(props: {
  label: string
  children: ReactNode
  hint?: string
  wide?: boolean
}) {
  useLocale()
  return (
    <div className={`ops-field ${props.wide ? 'ops-field-wide' : ''}`}>
      <label className='ops-field-control'>
        <span>{props.label}</span>
        {props.children}
      </label>
      {props.hint && <small>{props.hint}</small>}
    </div>
  )
}
export function RefreshButton(props: {
  pending: boolean
  onClick: () => void
}) {
  useLocale()
  return (
    <Button variant='outline' disabled={props.pending} onClick={props.onClick}>
      {translate('刷新')}
    </Button>
  )
}
export type Filters = Record<string, string>
function localInput(date: Date) {
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 16)
}
export function RequestFilters(props: {
  admin?: boolean
  detail?: boolean
  onApply: (filters: Filters) => void
  initial?: Filters
}) {
  useLocale()
  const [form, setForm] = useState<Filters>(() => {
    const initial = { ...props.initial }
    for (const k of ['from', 'to']) {
      if (initial[k] && !Number.isNaN(Date.parse(initial[k]))) {
        initial[k] = localInput(new Date(initial[k]))
      }
    }
    return initial
  })
  const catalog = useQuery({
    queryKey: ['catalog'],
    queryFn: () => request<Endpoint[]>('/catalog'),
  })
  const set = (name: string, value: string) =>
    setForm({ ...form, [name]: value })
  const [days, setDays] = useState(() => {
    if (!props.initial?.from || !props.initial?.to) return '7'
    const duration =
      (Date.parse(props.initial.to) - Date.parse(props.initial.from)) / 86400000
    for (const candidate of [1, 7, 30, 59]) {
      if (Math.abs(duration - candidate) < 0.002) return String(candidate)
    }
    return 'custom'
  })
  const [advanced, setAdvanced] = useState(
    days === 'custom' ||
      !!props.initial?.error_code ||
      !!props.initial?.request_id
  )
  function apply(e: FormEvent) {
    e.preventDefault()
    const value = { ...form }
    for (const key of ['from', 'to']) {
      if (value[key]) value[key] = new Date(value[key]).toISOString()
    }
    props.onApply(value)
  }
  function preset(value: string) {
    setDays(value)
    if (value === 'custom') {
      setAdvanced(true)
      return
    }
    const next = {
      ...form,
      from: localInput(new Date(Date.now() - Number(value) * 86400000)),
      to: localInput(new Date()),
    }
    setForm(next)
    props.onApply({
      ...next,
      from: new Date(next.from).toISOString(),
      to: new Date(next.to).toISOString(),
    })
  }
  return (
    <form className='ops-filters' onSubmit={apply}>
      <div className='ops-filter-grid'>
        <Field label={translate('时间范围')}>
          <select value={days} onChange={(e) => preset(e.target.value)}>
            <option value='1'>{translate('最近 24 小时')}</option>
            <option value='7'>{translate('最近 7 天')}</option>
            <option value='30'>{translate('最近 30 天')}</option>
            <option value='59'>{translate('最近 59 天')}</option>
            <option value='custom'>{translate('自定义时间')}</option>
          </select>
        </Field>
        <Field label={translate('接口')}>
          <select
            value={form.endpoint || ''}
            onChange={(e) => set('endpoint', e.target.value)}
          >
            <option value=''>{translate('全部接口')}</option>
            {catalog.data?.map((e) => (
              <option key={e.path} value={e.path}>
                {translate(e.name)}
              </option>
            ))}
          </select>
        </Field>
        <Field label={translate('应用编号')}>
          <Input
            inputMode='numeric'
            pattern='[0-9]*'
            value={form.token_id || ''}
            placeholder={translate('全部应用')}
            onChange={(e) => set('token_id', e.target.value)}
          />
        </Field>
        {props.admin && (
          <Field label={translate('用户编号')}>
            <Input
              inputMode='numeric'
              pattern='[0-9]*'
              value={form.user_id || ''}
              placeholder={translate('全部用户')}
              onChange={(e) => set('user_id', e.target.value)}
            />
          </Field>
        )}
        <Field label={translate('请求状态')}>
          <select
            value={form.status || ''}
            onChange={(e) => set('status', e.target.value)}
          >
            <option value=''>{translate('全部状态')}</option>
            <option value='success'>{translate('成功')}</option>
            <option value='error'>{translate('全部错误')}</option>
            <option value='rejected'>{translate('未接纳')}</option>
            <option value='429'>{translate('限流 / 额度')}</option>
            <option value='4xx'>4xx</option>
            <option value='5xx'>5xx</option>
          </select>
        </Field>
      </div>
      <details
        className='ops-advanced-filters'
        open={advanced}
        onToggle={(e) => setAdvanced(e.currentTarget.open)}
      >
        <summary>
          {props.detail ? translate('更多筛选') : translate('自定义时间')}
        </summary>
        <div className='ops-filter-grid'>
          <Field label={translate('开始时间')}>
            <Input
              type='datetime-local'
              value={form.from || ''}
              onChange={(e) => {
                set('from', e.target.value)
                setDays('custom')
              }}
            />
          </Field>
          <Field label={translate('结束时间')}>
            <Input
              type='datetime-local'
              value={form.to || ''}
              onChange={(e) => {
                set('to', e.target.value)
                setDays('custom')
              }}
            />
          </Field>
          {props.detail && (
            <>
              <Field label={translate('请求编号')}>
                <Input
                  value={form.request_id || ''}
                  onChange={(e) => set('request_id', e.target.value)}
                />
              </Field>
              <Field label={translate('上游请求编号')}>
                <Input
                  value={form.upstream_request_id || ''}
                  onChange={(e) => set('upstream_request_id', e.target.value)}
                />
              </Field>
              <Field label={translate('错误码')}>
                <Input
                  value={form.error_code || ''}
                  onChange={(e) => set('error_code', e.target.value)}
                />
              </Field>
              <Field label={translate('最小耗时（ms）')}>
                <Input
                  type='number'
                  min='0'
                  max='120000'
                  value={form.min_ms || ''}
                  onChange={(e) => set('min_ms', e.target.value)}
                />
              </Field>
              <Field label={translate('最大耗时（ms）')}>
                <Input
                  type='number'
                  min='0'
                  max='120000'
                  value={form.max_ms || ''}
                  onChange={(e) => set('max_ms', e.target.value)}
                />
              </Field>
            </>
          )}
        </div>
      </details>
      <div className='ops-actions'>
        <Button type='submit'>{translate('筛选')}</Button>
        <Button
          variant='outline'
          type='button'
          onClick={() => {
            setForm({})
            setDays('7')
            props.onApply({})
          }}
        >
          {translate('重置')}
        </Button>
      </div>
    </form>
  )
}
