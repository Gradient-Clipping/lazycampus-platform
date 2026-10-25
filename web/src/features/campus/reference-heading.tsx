import { ArrowDownToLine } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { currentLanguage, translate, useLocale } from '@/i18n/translate'

export function ReferenceHeading({ children }: { children: ReactNode }) {
  useLocale()
  const [pending, setPending] = useState(false)
  async function download() {
    setPending(true)
    try {
      const response = await fetch(
        `/openapi.json?lang=${currentLanguage()}&download=1`,
        {
          headers: { Accept: 'application/json' },
          cache: 'no-store',
        }
      )
      if (
        !response.ok ||
        !response.headers.get('Content-Type')?.includes('application/json')
      ) {
        throw new Error('Invalid OpenAPI response')
      }
      const spec: unknown = await response.json()
      if (
        !spec ||
        typeof spec !== 'object' ||
        !('openapi' in spec) ||
        !('paths' in spec)
      ) {
        throw new Error('Invalid OpenAPI document')
      }
      const url = URL.createObjectURL(
        new Blob([JSON.stringify(spec, null, 2)], {
          type: 'application/json;charset=utf-8',
        })
      )
      const link = document.createElement('a')
      link.href = url
      link.download = 'lazycampus-openapi.json'
      link.click()
      window.setTimeout(() => URL.revokeObjectURL(url), 1000)
    } catch {
      toast.error(translate('无法下载 OpenAPI，请稍后重试。'))
    } finally {
      setPending(false)
    }
  }
  return (
    <div className='reference-document-title'>
      <h1>{children}</h1>
      <Button
        variant='outline'
        disabled={pending}
        onClick={() => void download()}
      >
        <ArrowDownToLine size={16} />
        {translate(pending ? '正在下载…' : '下载 OpenAPI')}
      </Button>
    </div>
  )
}
