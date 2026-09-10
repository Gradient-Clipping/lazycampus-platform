import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useId, useMemo, useState } from 'react'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { translate, useLocale, dateLocale } from '@/i18n/translate'

import { request } from './api'
import { filterQuery } from './operations-data'
import {
  RequestFilters,
  RefreshButton,
  type Filters,
} from './operations-shared'
import { ErrorState, Loading, PageHeading } from './shared'
import type { Analytics, Metrics } from './types'

const dimensionField: Record<string, string> = {
  users: 'user_id',
  applications: 'token_id',
  endpoints: 'endpoint',
}
const percent = (errors: number, total: number) =>
  total ? `${((errors / total) * 100).toFixed(1)}%` : '0%'
export function UsageAnalytics({ admin = false }: { admin?: boolean }) {
  useLocale()
  const [filters, setFilters] = useState<Filters>({})
  const [dimension, setDimension] = useState(admin ? 'users' : 'applications')
  const data = useQuery({
    queryKey: ['analytics', admin, filters],
    queryFn: () =>
      request<Analytics>(
        `${admin ? '/admin' : ''}/analytics?${filterQuery(filters)}`
      ),
    placeholderData: keepPreviousData,
  })
  const m = data.data?.summary
  const drill = (key: string) =>
    setFilters({ ...filters, [dimensionField[dimension]]: key })
  return (
    <>
      <PageHeading
        title={admin ? translate('全站运营看板') : translate('用量分析')}
        action={
          <RefreshButton
            pending={data.isFetching}
            onClick={() => void data.refetch()}
          />
        }
      />
      <RequestFilters
        key={JSON.stringify(filters)}
        initial={filters}
        admin={admin}
        onApply={setFilters}
      />
      {data.isPending && <Loading />}
      {data.error && (
        <ErrorState error={data.error} retry={() => void data.refetch()} />
      )}
      {data.data && m && (
        <div aria-busy={data.isFetching} className='ops-stack'>
          <div className='ops-metrics'>
            {[
              [
                translate('请求总量'),
                m.requests.toLocaleString(),
                translate('{{v0}} 次已接纳', {
                  v0: m.admitted.toLocaleString(),
                }),
              ],
              [
                translate('错误率'),
                percent(m.errors, m.requests),
                translate('{{v0}} 次错误', { v0: m.errors.toLocaleString() }),
              ],
              [
                translate('平均耗时'),
                `${Math.round(m.avg_ms)} ms`,
                `P95 ${data.data.p95_ms} ms`,
              ],
              [
                translate('限流 / 额度'),
                m.limited.toLocaleString(),
                'HTTP 429',
              ],
              [
                translate('未接纳'),
                m.rejected.toLocaleString(),
                translate('未计入调用额度'),
              ],
              [
                translate('服务错误'),
                m.server_errors.toLocaleString(),
                'HTTP 5xx',
              ],
            ].map(([label, value, detail]) => (
              <Card key={label} className='ops-metric'>
                <span>{label}</span>
                <strong>{value}</strong>
                <small>{detail}</small>
              </Card>
            ))}
          </div>
          <Trend data={data.data} />
          <Card className='table-card'>
            <div className='ops-card-heading'>
              <h2>{translate('分组统计')}</h2>
              <div className='segmented-tabs'>
                {(admin
                  ? ['users', 'applications', 'endpoints']
                  : ['applications', 'endpoints']
                ).map((key) => (
                  <button
                    key={key}
                    type='button'
                    aria-pressed={dimension === key}
                    onClick={() => setDimension(key)}
                  >
                    {
                      {
                        users: translate('用户'),
                        applications: translate('应用'),
                        endpoints: translate('接口'),
                      }[key]
                    }
                  </button>
                ))}
              </div>
            </div>
            <StaticDataTable
              data={data.data.groups[dimension] || []}
              getRowKey={(row) => row.key}
              emptyContent={translate('当前筛选下没有调用记录')}
              columns={[
                {
                  id: 'name',
                  header: translate('名称 / 编号'),
                  cell: (row) => (
                    <button
                      className='ops-text-button'
                      type='button'
                      onClick={() => drill(row.key)}
                    >
                      {row.name ||
                        (row.key === '0' ? translate('未识别') : `#${row.key}`)}
                      {dimension !== 'endpoints' && row.name && (
                        <small> #{row.key}</small>
                      )}
                    </button>
                  ),
                },
                {
                  id: 'requests',
                  header: translate('请求'),
                  cell: (row) => row.requests.toLocaleString(),
                },
                {
                  id: 'errors',
                  header: translate('错误率'),
                  cell: (row) => percent(row.errors, row.requests),
                },
                {
                  id: 'avg',
                  header: translate('平均耗时'),
                  cell: (row) => `${Math.round(row.avg_ms)} ms`,
                },
                {
                  id: 'limited',
                  header: translate('限流'),
                  cell: (row) => row.limited,
                },
                {
                  id: 'rejected',
                  header: translate('未接纳'),
                  cell: (row) => row.rejected,
                },
                {
                  id: 'logs',
                  header: translate('详情'),
                  cell: (row) => (
                    <a
                      className='ops-text-link'
                      href={`${admin ? '/admin' : ''}/logs?${filterQuery({ ...filters, [dimensionField[dimension]]: row.key })}`}
                    >
                      {translate('调用日志')}
                    </a>
                  ),
                },
              ]}
            />
            <p className='ops-footnote'>
              {translate('按请求数展示前 ')}
              {data.data.group_limit}{' '}
              {translate('项；错误率包含权限与限流拒绝。')}
            </p>
          </Card>
        </div>
      )}
    </>
  )
}
function Trend({ data }: { data: Analytics }) {
  useLocale()
  const [metric, setMetric] = useState<
    'requests' | 'errors' | 'limited' | 'avg_ms'
  >('requests')
  const [active, setActive] = useState<number | null>(null)
  const gradient = useId().replaceAll(':', '')
  const points = useMemo(() => {
    const step = data.bucket === 'hour' ? 3600000 : 86400000
    const start =
      Math.floor(new Date(data.window.start).getTime() / step) * step
    const end = new Date(data.window.end).getTime()
    const byTime = new Map(
      data.series.map((row) => [new Date(row.time).getTime(), row])
    )
    const result: (Metrics & { time: string })[] = []
    for (let time = start; time < end; time += step) {
      result.push(
        byTime.get(time) || {
          time: new Date(time).toISOString(),
          requests: 0,
          admitted: 0,
          errors: 0,
          limited: 0,
          rejected: 0,
          server_errors: 0,
          avg_ms: 0,
          max_ms: 0,
        }
      )
    }
    return result
  }, [data])
  const max = Math.max(1, ...points.map((p) => p[metric]))
  const position = (index: number) => ({
    x: 42 + (index / Math.max(1, points.length - 1)) * 820,
    y: 210 - (points[index][metric] / max) * 166,
  })
  const line = points
    .map((_, index) => {
      const p = position(index)
      return `${p.x},${p.y}`
    })
    .join(' ')
  const format = (value: string) =>
    new Date(value).toLocaleString(dateLocale(), {
      month: 'short',
      day: 'numeric',
      ...(data.bucket === 'hour' ? { hour: '2-digit' } : {}),
    })
  const labels = {
    requests: translate('调用量'),
    errors: translate('错误'),
    limited: translate('限流'),
    avg_ms: translate('平均耗时'),
  }
  const selected = active !== null ? points[active] : undefined
  return (
    <Card className='ops-chart'>
      <div className='ops-card-heading'>
        <div>
          <h2>{translate('调用趋势')}</h2>
          <p>
            {data.bucket === 'hour' ? translate('每小时') : translate('每日')}
            {translate(' · 本地时间')}
          </p>
        </div>
        <div className='segmented-tabs'>
          {Object.entries(labels).map(([key, label]) => (
            <button
              key={key}
              type='button'
              aria-pressed={metric === key}
              onClick={() => {
                setMetric(key as typeof metric)
                setActive(null)
              }}
            >
              {label}
            </button>
          ))}
        </div>
      </div>
      <div className='ops-chart-plot'>
        <svg
          viewBox='0 0 900 250'
          role='img'
          aria-label={translate('{{v0}}趋势', { v0: labels[metric] })}
        >
          <defs>
            <linearGradient id={gradient} x1='0' y1='0' x2='0' y2='1'>
              <stop offset='0%' stopColor='var(--chart-1)' stopOpacity='.23' />
              <stop
                offset='100%'
                stopColor='var(--chart-1)'
                stopOpacity='.015'
              />
            </linearGradient>
          </defs>
          {[0, 0.5, 1].map((r) => (
            <g key={r}>
              <line
                x1='42'
                x2='862'
                y1={210 - r * 166}
                y2={210 - r * 166}
                stroke='var(--border)'
                strokeDasharray='4 5'
              />
              <text x='30' y={214 - r * 166} textAnchor='end'>
                {Math.round(max * r)}
              </text>
            </g>
          ))}
          {points.length > 0 && (
            <>
              <polygon
                points={`42,210 ${line} 862,210`}
                fill={`url(#${gradient})`}
              />
              <polyline
                points={line}
                fill='none'
                stroke='var(--chart-1)'
                strokeWidth='2.5'
                strokeLinejoin='round'
              />
            </>
          )}
          {points.map((p, index) => (
            <circle
              key={p.time}
              {...{ cx: position(index).x, cy: position(index).y }}
              r={active === index ? 5 : 3}
              className='ops-chart-point'
              tabIndex={0}
              role='button'
              aria-label={`${format(p.time)} ${labels[metric]} ${Math.round(p[metric])}`}
              onFocus={() => setActive(index)}
              onMouseEnter={() => setActive(index)}
              onKeyDown={(e) => {
                if (e.key === 'Escape') setActive(null)
              }}
            >
              <title>
                {format(p.time)}：{Math.round(p[metric])}
              </title>
            </circle>
          ))}
          {points.length > 0 &&
            [0, Math.floor((points.length - 1) / 2), points.length - 1]
              .filter((v, i, a) => a.indexOf(v) === i)
              .map((i) => (
                <text
                  key={i}
                  x={position(i).x}
                  y='240'
                  textAnchor={i === 0 ? 'start' : 'end'}
                >
                  {format(points[i].time)}
                </text>
              ))}
        </svg>
      </div>
      <div className='ops-chart-reading' aria-live='polite'>
        {selected
          ? `${format(selected.time)} · ${labels[metric]} ${Math.round(selected[metric])}${metric === 'avg_ms' ? ' ms' : ''}`
          : translate('移动到图表节点或使用 Tab 查看数据')}
        {selected && (
          <Button size='sm' variant='ghost' onClick={() => setActive(null)}>
            {translate('清除')}
          </Button>
        )}
      </div>
    </Card>
  )
}
