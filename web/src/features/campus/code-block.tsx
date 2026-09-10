import type { ReactNode } from 'react'

// Render tokens as React text, including untrusted responses from the API explorer.
// The original source is preserved exactly for selection and copying.
export function CodeBlock({
  code,
  language = 'bash',
  className = '',
}: {
  code: string
  language?: 'bash' | 'json' | 'javascript' | 'python'
  className?: string
}) {
  let pattern =
    /\/\/[^\n]*|#[^\n]*|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|`(?:\\.|[^`\\])*`|\b(?:const|let|await|async|if|throw|new|import|from|try|except|with|as|raise|return|else)\b|\b(?:true|false|null|True|False|None)\b|\b\d+\b|[{}()[\],:;.]/g
  if (language === 'json') {
    pattern =
      /"(?:\\.|[^"\\])*"(?=\s*:)|"(?:\\.|[^"\\])*"|-?\b\d+(?:\.\d+)?(?:[eE][+-]?\d+)?\b|\b(?:true|false|null)\b|[{}[\],:]/g
  } else if (language === 'bash') {
    pattern =
      /"(?:\\.|[^"\\])*"|'[^']*'|\bcurl\b|--?[A-Za-z][\w-]*|\b(?:GET|POST|PATCH|PUT|DELETE)\b|\\/g
  }
  const tokens: ReactNode[] = []
  let cursor = 0
  for (const match of code.matchAll(pattern)) {
    const index = match.index
    tokens.push(code.slice(cursor, index))
    const value = match[0]
    let type = 'punctuation'
    if (value.startsWith('//') || value.startsWith('#')) {
      type = 'comment'
    } else if (
      value.startsWith('"') ||
      value.startsWith("'") ||
      value.startsWith('`')
    ) {
      type =
        language === 'json' && /^\s*:/.test(code.slice(index + value.length))
          ? 'key'
          : 'string'
    } else if (language === 'bash') {
      if (value === 'curl') type = 'command'
      else if (value !== '\\') type = 'flag'
    } else if (/^(true|false|null|True|False|None)$/.test(value)) {
      type = 'literal'
    } else if (/^-?\d/.test(value)) type = 'number'
    else if (/^[a-z]+$/.test(value)) type = 'command'
    tokens.push(
      <span key={index} className={`syntax-${type}`}>
        {value}
      </span>
    )
    cursor = index + value.length
  }
  tokens.push(code.slice(cursor))
  return (
    <pre className={`syntax-code ${className}`}>
      <code>{tokens}</code>
    </pre>
  )
}
