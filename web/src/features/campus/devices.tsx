import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Monitor, Smartphone } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { translate, useLocale } from '@/i18n/translate'

import { formatLoginDate, request } from './api'
import { RefreshButton } from './operations-shared'
import { ErrorState, Loading, PageHeading } from './shared'
import type { SessionDevice } from './types'

export function Devices() {
  useLocale()
  const cache = useQueryClient(),
    [target, setTarget] = useState<SessionDevice | 'others' | null>(null),
    [pending, setPending] = useState(false)
  const devices = useQuery({
    queryKey: ['sessions'],
    queryFn: () =>
      request<{ items: SessionDevice[]; current_id: string }>('/sessions'),
  })
  async function revoke() {
    if (!target) return
    setPending(true)
    try {
      const result = await request<{ current?: boolean; sso_revoked: boolean }>(
        target === 'others'
          ? '/sessions/revoke-others'
          : `/sessions/${target.id}`,
        target === 'others' ? 'POST' : 'DELETE'
      )
      if (result.current) {
        cache.clear()
        window.location.assign('/signed-out')
        return
      }
      await cache.invalidateQueries({ queryKey: ['sessions'] })
      setTarget(null)
      if (result.sso_revoked) toast.success(translate('设备已退出登录'))
      else toast.warning(translate('平台会话已退出，统一登录会话正在重试退出'))
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setPending(false)
    }
  }
  return (
    <>
      <PageHeading
        title={translate('登录设备')}
        action={
          <div className='ops-actions'>
            <RefreshButton
              pending={devices.isFetching}
              onClick={() => void devices.refetch()}
            />
            <Button
              variant='outline'
              disabled={(devices.data?.items.length || 0) < 2}
              onClick={() => setTarget('others')}
            >
              {translate('退出其他设备')}
            </Button>
          </div>
        }
      />
      {devices.isPending && <Loading />}
      {devices.error && <ErrorState error={devices.error} />}
      {devices.data && (
        <div className='ops-stack'>
          {devices.data.items.map((d) => (
            <Card key={d.id} className='ops-device'>
              <span className='ops-device-icon'>
                {/Android|iOS/.test(d.device) ? <Smartphone /> : <Monitor />}
              </span>
              <div className='ops-device-info'>
                <div>
                  <h2>{translate(d.device)}</h2>
                  {d.id === devices.data.current_id && (
                    <Badge variant='secondary'>{translate('当前设备')}</Badge>
                  )}
                </div>
                <p>{d.ip || translate('未知 IP')}</p>
                <small>
                  {translate('最近活动 ')}
                  {formatLoginDate(d.last_active_at)}
                  <br />
                  {translate('登录时间 ')}
                  {formatLoginDate(d.created_at)}
                </small>
              </div>
              <Button variant='outline' onClick={() => setTarget(d)}>
                {translate('退出')}
              </Button>
            </Card>
          ))}
        </div>
      )}
      <ConfirmDialog
        open={target !== null}
        onOpenChange={(open) => {
          if (!open && !pending) setTarget(null)
        }}
        title={
          target === 'others'
            ? translate('退出其他设备？')
            : translate('退出此设备？')
        }
        desc={
          target === 'others'
            ? translate('其他设备将退出登录，当前设备保持登录。')
            : translate('此设备将退出登录。')
        }
        confirmText={translate('退出设备')}
        isLoading={pending}
        handleConfirm={() => void revoke()}
      />
    </>
  )
}
