import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { translate } from '@/i18n/translate'

export function PageHeading(props: {
  title: string
  description?: string
  action?: ReactNode
}) {
  return (
    <div className='page-heading'>
      <div>
        <h1>{props.title}</h1>
        {props.description && <p>{props.description}</p>}
      </div>
      {props.action}
    </div>
  )
}
export function Loading() {
  const { t } = useTranslation()
  return (
    <div aria-label={t('Loading')} role='status' className='grid gap-5'>
      <Skeleton className='h-12 w-48' />
      <Skeleton className='h-52 w-full' />
    </div>
  )
}
export function ErrorState(props: { error: Error; retry?: () => void }) {
  const { t } = useTranslation()
  return (
    <div role='alert' className='empty-state'>
      <p>{translate(props.error.message)}</p>
      {props.retry && (
        <Button variant='outline' onClick={props.retry}>
          {t('Retry')}
        </Button>
      )}
    </div>
  )
}
export function Pager(props: {
  page: number
  total: number
  size?: number
  onChange: (page: number) => void
}) {
  const { t } = useTranslation()
  const size = props.size || 25
  return (
    <div className='pager'>
      <span>
        {props.total.toLocaleString()} · {props.page} /{' '}
        {Math.max(1, Math.ceil(props.total / size))}
      </span>
      <div>
        <Button
          variant='outline'
          disabled={props.page <= 1}
          onClick={() => props.onChange(props.page - 1)}
        >
          {t('Previous')}
        </Button>
        <Button
          variant='outline'
          disabled={props.page * size >= props.total}
          onClick={() => props.onChange(props.page + 1)}
        >
          {t('Next')}
        </Button>
      </div>
    </div>
  )
}
