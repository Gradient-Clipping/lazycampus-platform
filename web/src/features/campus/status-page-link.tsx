import { ArrowUpRight } from 'lucide-react'

import { translate, useLocale } from '@/i18n/translate'

export function StatusPageLink() {
  useLocale()
  return (
    <a
      className='service-status-link'
      href='https://status.lazycampus.com'
      target='_blank'
      rel='noopener noreferrer'
    >
      {translate('全站服务状态')}
      <ArrowUpRight size={14} aria-hidden='true' />
    </a>
  )
}
