import { useState } from 'react'

import { CopyButton } from '@/components/copy-button'
import { translate, useLocale } from '@/i18n/translate'

import { CodeBlock } from './code-block'
import { requestExample, type ExampleLanguage } from './reference-data'

export function RequestExample({
  path,
  binary = false,
}: {
  path: string
  binary?: boolean
}) {
  useLocale()
  const [language, setLanguage] = useState<ExampleLanguage>('bash')
  const code = requestExample(path, binary, language)
  return (
    <div className='code-example reference-code'>
      <div>
        <div
          className='reference-code-tabs'
          role='group'
          aria-label={translate('示例语言')}
        >
          {(
            [
              ['bash', 'cURL · Bash'],
              ['javascript', 'Node.js'],
              ['python', 'Python'],
            ] as const
          ).map(([value, label]) => (
            <button
              type='button'
              key={value}
              aria-pressed={language === value}
              onClick={() => setLanguage(value)}
            >
              {label}
            </button>
          ))}
        </div>
        <CopyButton value={code} />
      </div>
      <CodeBlock code={code} language={language} />
    </div>
  )
}
