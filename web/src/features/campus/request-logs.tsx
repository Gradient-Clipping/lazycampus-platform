import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useState } from 'react'

import { CopyButton } from '@/components/copy-button'
import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { Dialog } from '@/components/dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { translate, useLocale } from '@/i18n/translate'

import { formatLoginDate, request } from './api'
import { filterQuery } from './operations-data'
import {
  RequestFilters,
  RefreshButton,
  type Filters,
} from './operations-shared'
import { ErrorState, Loading, PageHeading, Pager } from './shared'
import type { Page, RequestLog } from './types'

export function Logs({ admin = false }: { admin?: boolean }) {
  useLocale()
  const [page, setPage] = useState(1)
  const [filters, setFilters] = useState<Filters>(() =>
    Object.fromEntries(new URLSearchParams(window.location.search))
  )
  const [selected, setSelected] = useState<number | null>(null)
  const base = admin ? '/admin' : ''
  const logs = useQuery({
    queryKey: ['logs', admin, page, filters],
    queryFn: () =>
      request<Page<RequestLog>>(
        `${base}/logs?page=${page}&${filterQuery(filters)}`
      ),
    placeholderData: keepPreviousData,
  })
  const detail = useQuery({
    queryKey: ['log-detail', admin, selected],
    queryFn: () =>
      request<{
        log: RequestLog
        user: { username: string; display_name: string }
        application: { name: string; exists: boolean }
      }>(`${base}/logs/${selected}`),
    enabled: selected !== null,
  })
  return (
    <>
      <PageHeading
        title={admin ? translate('全站调用日志') : translate('调用日志')}
        action={
          <RefreshButton
            pending={logs.isFetching}
            onClick={() => void logs.refetch()}
          />
        }
      />
      <RequestFilters
        admin={admin}
        detail
        initial={filters}
        onApply={(v) => {
          setFilters(v)
          setPage(1)
        }}
      />
      {logs.isPending && <Loading />}
      {logs.error && <ErrorState error={logs.error} />}
      {logs.data && (
        <Card className='table-card' aria-busy={logs.isFetching}>
          <StaticDataTable
            data={logs.data.items}
            getRowKey={(row) => row.id}
            emptyContent={translate('当前筛选下没有调用记录')}
            columns={[
              {
                id: 'time',
                header: translate('时间'),
                cell: (row) => (
                  <span className='nowrap'>
                    {formatLoginDate(row.created_at)}
                  </span>
                ),
              },
              {
                id: 'endpoint',
                header: translate('接口'),
                cell: (row) => (
                  <code className='endpoint-cell'>{row.endpoint}</code>
                ),
              },
              {
                id: 'app',
                header: translate('应用 / 用户'),
                cell: (row) => (
                  <div>
                    {row.token_name || `#${row.token_id}`}
                    <small className='ops-block'>
                      {admin &&
                        (row.user_name ||
                          translate('用户 #{{v0}}', { v0: row.user_id }))}
                    </small>
                  </div>
                ),
              },
              {
                id: 'status',
                header: translate('状态'),
                cell: (row) => (
                  <Badge
                    variant={row.status >= 400 ? 'destructive' : 'secondary'}
                  >
                    {row.status || translate('进行中')}
                  </Badge>
                ),
              },
              {
                id: 'duration',
                header: translate('耗时'),
                cell: (row) => (
                  <span className='nowrap'>{row.duration_ms} ms</span>
                ),
              },
              {
                id: 'reason',
                header: translate('错误 / 拒绝原因'),
                cell: (row) => (
                  <span title={translate(row.error_message)}>
                    {row.error_code || '—'}
                  </span>
                ),
              },
              {
                id: 'detail',
                header: translate('详情'),
                cell: (row) => (
                  <Button
                    size='sm'
                    variant='ghost'
                    onClick={() => setSelected(row.id)}
                  >
                    {translate('查看')}
                  </Button>
                ),
              },
            ]}
          />
          <Pager page={page} total={logs.data.total} onChange={setPage} />
        </Card>
      )}
      <Dialog
        open={selected !== null}
        onOpenChange={(open) => {
          if (!open) setSelected(null)
        }}
        title={translate('请求详情')}
      >
        {detail.isPending && <Loading />}
        {detail.error && <ErrorState error={detail.error} />}
        {detail.data && (
          <div className='ops-stack'>
            <dl className='ops-detail'>
              {[
                [
                  translate('时间'),
                  formatLoginDate(detail.data.log.created_at),
                ],
                [
                  translate('接口'),
                  `${detail.data.log.method} /v1${detail.data.log.endpoint}`,
                ],
                [translate('HTTP 状态'), String(detail.data.log.status)],
                [
                  translate('计入额度'),
                  detail.data.log.admitted ? translate('是') : translate('否'),
                ],
                [translate('耗时'), `${detail.data.log.duration_ms} ms`],
                [
                  translate('用户'),
                  `${detail.data.log.user_name || detail.data.user.username || translate('未识别')} (#${detail.data.log.user_id})`,
                ],
                [
                  translate('应用'),
                  `${detail.data.log.token_name || detail.data.application.name || translate('未识别')} (#${detail.data.log.token_id})${detail.data.log.token_id && !detail.data.application.exists ? translate(' · 已删除') : ''}`,
                ],
                [translate('来源 IP'), detail.data.log.client_ip || '—'],
                [translate('错误码'), detail.data.log.error_code || '—'],
                [
                  translate('触发限制'),
                  (
                    {
                      account_minute: translate('账户每分钟速率'),
                      application_minute: translate('应用每分钟速率'),
                      account_burst: translate('账户突发限制'),
                      endpoint_minute: translate('接口每分钟速率'),
                      endpoint_cooldown: translate('接口冷却间隔'),
                      user_daily: translate('账户每日额度'),
                      token_daily: translate('应用每日额度'),
                      endpoint_daily: translate('接口每日额度'),
                      application_total: translate('应用总额度'),
                      concurrency: translate('同时请求数'),
                    } as Record<string, string>
                  )[detail.data.log.limit_scope || ''] || '—',
                ],
                [
                  translate('错误信息'),
                  translate(detail.data.log.error_message) || '—',
                ],
                [
                  translate('建议等待'),
                  detail.data.log.retry_after
                    ? translate('{{v0}} 秒', {
                        v0: detail.data.log.retry_after,
                      })
                    : '—',
                ],
              ].map(([key, value]) => (
                <div key={key}>
                  <dt>{key}</dt>
                  <dd>{value}</dd>
                </div>
              ))}
            </dl>
            <div className='ops-id-row'>
              <span>{translate('请求编号')}</span>
              <code>{detail.data.log.request_id}</code>
              <CopyButton value={detail.data.log.request_id} />
            </div>
            {detail.data.log.upstream_request_id && (
              <div className='ops-id-row'>
                <span>{translate('上游请求编号')}</span>
                <code>{detail.data.log.upstream_request_id}</code>
                <CopyButton value={detail.data.log.upstream_request_id} />
              </div>
            )}
          </div>
        )}
      </Dialog>
    </>
  )
}
