import { useQuery } from '@tanstack/react-query'
import { Link, useRouterState } from '@tanstack/react-router'
import {
  ArrowDownToLine,
  ArrowRight,
  BookOpen,
  Search,
  Send,
} from 'lucide-react'
import { useMemo, useState } from 'react'

import { CopyButton } from '@/components/copy-button'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  translate,
  useLocale,
  translateReference,
  currentLanguage,
} from '@/i18n/translate'

import { request } from './api'
import { CodeBlock } from './code-block'
import { statusLabels } from './operations-data'
import { scopeName } from './permissions'
import { RequestExample } from './reference-code'
import { apiOrigin, referenceGuides, schemaType } from './reference-data'
import { ReferenceGuide } from './reference-guide'
import { ErrorState, Loading, PageHeading } from './shared'
import type { Endpoint, ReferenceSchema } from './types'

export function ApiReference() {
  const locale = useLocale()
  const catalog = useQuery({
    queryKey: ['catalog'],
    queryFn: () => request<Endpoint[]>('/catalog'),
  })
  const localized = useMemo(
    () =>
      (catalog.data || []).map((entry) => ({
        ...entry,
        name: translate(entry.name),
        description: translate(entry.description),
        message: translate(entry.message),
        documentation: translateReference(entry.documentation, locale),
      })),
    [catalog.data, locale]
  )
  const hash = useRouterState({ select: (state) => state.location.hash })
  const [search, setSearch] = useState('')
  if (catalog.isPending) return <Loading />
  if (catalog.error) {
    return (
      <ErrorState error={catalog.error} retry={() => void catalog.refetch()} />
    )
  }
  const active = localized.find((e) => `api${e.path}` === hash)
  const guide = referenceGuides.find((g) => g.id === hash)?.id || 'start'
  const endpoints = localized.filter((e) =>
    `${e.name} ${e.path} ${e.scope} ${e.documentation?.group || ''}`
      .toLowerCase()
      .includes(search.trim().toLowerCase())
  )
  const groups = [
    ...new Set(
      endpoints.map((e) => e.documentation?.group || translate('接口目录'))
    ),
  ]
  return (
    <>
      <PageHeading
        title={translate('接口文档')}
        description={translate('接入指南、接口契约与在线调试。')}
        action={
          <Button
            variant='outline'
            render={
              <a
                href={`/openapi.json?lang=${currentLanguage()}`}
                download='lazycampus-openapi.json'
              />
            }
          >
            <ArrowDownToLine size={16} />
            {translate('下载 OpenAPI')}
          </Button>
        }
      />
      <div className='reference-layout reference-v2'>
        <aside className='api-list'>
          <div className='search-field'>
            <Search size={16} />
            <Input
              aria-label={translate('搜索接口')}
              placeholder={translate('搜索名称、路径或权限')}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
            {search && (
              <button
                type='button'
                className='reference-clear'
                onClick={() => setSearch('')}
              >
                {translate('清除')}
              </button>
            )}
          </div>
          <nav className='api-list-scroll' aria-label={translate('文档目录')}>
            <p className='reference-nav-title'>{translate('接入指南')}</p>
            {referenceGuides.map((g) => (
              <Link
                key={g.id}
                to='/api-reference'
                activeOptions={{ includeHash: true }}
                hash={g.id}
                aria-current={!active && g.id === guide ? 'page' : undefined}
                className={`api-item reference-guide-item${!active && g.id === guide ? ' selected' : ''}`}
              >
                <span>{translate(g.title)}</span>
              </Link>
            ))}
            <div className='reference-nav-divider' />
            {groups.map((group) => (
              <div key={group}>
                <p className='reference-nav-title'>{group}</p>
                {endpoints
                  .filter(
                    (e) =>
                      (e.documentation?.group || translate('接口目录')) ===
                      group
                  )
                  .map((e) => (
                    <Link
                      key={e.path}
                      to='/api-reference'
                      activeOptions={{ includeHash: true }}
                      hash={`api${e.path}`}
                      className={`api-item${e.path === active?.path ? ' selected' : ''}`}
                      aria-current={
                        e.path === active?.path ? 'page' : undefined
                      }
                    >
                      <span>{e.name}</span>
                      <code>GET {e.path}</code>
                    </Link>
                  ))}
              </div>
            ))}
            {!endpoints.length && (
              <p className='reference-empty' role='status'>
                {translate('没有匹配的接口，请尝试其他关键词。')}
              </p>
            )}
          </nav>
          <Link to='/status' className='reference-status-link'>
            {translate('查看接口状态 ')}
            <ArrowRight size={14} />
          </Link>
        </aside>
        <article
          className='api-document'
          aria-label={
            active?.name ||
            translate(referenceGuides.find((g) => g.id === guide)?.title || '')
          }
        >
          {hash && !active && !referenceGuides.some((g) => g.id === hash) && (
            <p className='reference-callout'>
              {translate('未找到此文档条目，请从目录选择接口。')}
            </p>
          )}
          {active ? (
            <EndpointReference
              key={active.path}
              endpoint={active}
              endpoints={localized}
            />
          ) : (
            <ReferenceGuide
              id={guide}
              quickstartScope={
                catalog.data.find((e) => e.path === '/content/feed')?.scope ||
                'content:read'
              }
            />
          )}
          <footer className='reference-footer'>
            <BookOpen size={15} />
            <span>{translate('API v1 · 只读校园接口')}</span>
            <a
              href={`/openapi.json?lang=${currentLanguage()}`}
              download='lazycampus-openapi.json'
            >
              OpenAPI 3.0.3 <ArrowDownToLine size={13} />
            </a>
          </footer>
        </article>
      </div>
    </>
  )
}

function SchemaFields({
  schema,
  prefix = '',
  depth = 0,
}: {
  schema: ReferenceSchema
  prefix?: string
  depth?: number
}) {
  useLocale()
  return (
    <div className='reference-fields'>
      {Object.entries(schema.properties || {}).map(([name, field]) => {
        const path = prefix ? `${prefix}.${name}` : name
        const child = field.type === 'array' ? field.items : field
        const childPrefix = field.type === 'array' ? `${path}[]` : path
        const children = Object.keys(child?.properties || {})
        return (
          <div className='reference-field' key={name}>
            <div>
              <code className='reference-field-name'>{path}</code>
              <span className='reference-field-type'>{schemaType(field)}</span>
            </div>
            {field.description && <p>{field.description}</p>}
            {field.enum && (
              <p>
                {translate('取值：')}
                <code>{field.enum.join(' · ')}</code>
              </p>
            )}
            {children.length > 0 && child && (
              <details open={depth === 0 && name === 'data'}>
                <summary>
                  {translate('查看 ')}
                  {children.length}
                  {translate(' 个字段')}
                </summary>
                <SchemaFields
                  schema={child}
                  prefix={childPrefix}
                  depth={depth + 1}
                />
              </details>
            )}
            {child?.additionalProperties &&
              typeof child.additionalProperties === 'object' && (
                <p>
                  {translate('值类型：')}
                  <code>{schemaType(child.additionalProperties)}</code>
                </p>
              )}
          </div>
        )
      })}
    </div>
  )
}

function EndpointReference({
  endpoint: e,
  endpoints,
}: {
  endpoint: Endpoint
  endpoints: Endpoint[]
}) {
  useLocale()
  const doc = e.documentation
  const [values, setValues] = useState<Record<string, string>>({})
  const query = new URLSearchParams(
    Object.entries(values).filter(([, value]) => value !== '')
  )
  const path = `/v1${e.path}${query.size ? `?${query.toString()}` : ''}`
  const exampleQuery = new URLSearchParams(
    e.parameters.flatMap((p) => {
      const value =
        values[p.name] ||
        (p.required
          ? String(doc?.parameters[p.name]?.example ?? p.example ?? '')
          : '')
      return value ? [[p.name, value]] : []
    })
  )
  const examplePath = `/v1${e.path}${exampleQuery.size ? `?${exampleQuery}` : ''}`
  const binary =
    doc?.content_type !== undefined && doc.content_type !== 'application/json'
  return (
    <>
      <p className='reference-eyebrow'>{doc?.group || translate('接口参考')}</p>
      <h2>{e.name}</h2>
      <p className='reference-lead'>{doc?.summary || e.description}</p>
      <div className='endpoint-heading'>
        <span className='method-badge'>GET</span>
        <code>/v1{e.path}</code>
        <CopyButton value={`${apiOrigin}/v1${e.path}`} />
      </div>
      <div className='reference-endpoint-meta'>
        <div>
          <span>{translate('所需权限')}</span>
          <strong>{scopeName(e.scope)}</strong>
          <code>{e.scope}</code>
        </div>
        <div>
          <span>{translate('响应格式')}</span>
          <code>{doc?.content_type || 'application/json'}</code>
        </div>
        <div>
          <span>{translate('当前状态')}</span>
          <strong className='ops-reference-status'>
            <span className={`ops-status-dot ${e.status || 'operational'}`} />
            {translate(statusLabels[e.status] || '正常')}
          </strong>
        </div>
      </div>
      {e.message && <p className='reference-callout'>{e.message}</p>}
      <p className='reference-limit-caption'>
        {translate('每用户此接口：')}
        {e.rate_limit}
        {translate(' 次/分钟 · ')}
        {e.daily_quota?.toLocaleString()} {translate('次/日')}
        {e.cooldown_seconds > 0
          ? translate(' · 请求间隔至少 {{v0}} 秒', { v0: e.cooldown_seconds })
          : ''}
        {translate('。')}
        <Link
          to='/api-reference'
          activeOptions={{ includeHash: true }}
          hash='limits'
        >
          {translate('账户与应用限制同时生效')}
        </Link>
        {translate('。')}
      </p>
      {doc && doc.notes.length > 0 && (
        <>
          <h3>{translate('调用说明')}</h3>
          <ul className='reference-notes'>
            {doc.notes.map((note) => (
              <li key={note}>{note}</li>
            ))}
          </ul>
        </>
      )}
      {doc && e.description && e.description !== doc.summary && (
        <details className='reference-operator-note'>
          <summary>{translate('运营说明')}</summary>
          <p>{e.description}</p>
        </details>
      )}
      <h3>{translate('查询参数')}</h3>
      {e.parameters.length ? (
        <div className='reference-table-wrap'>
          <table>
            <thead>
              <tr>
                <th>{translate('参数')}</th>
                <th>{translate('类型')}</th>
                <th>{translate('说明与约束')}</th>
              </tr>
            </thead>
            <tbody>
              {e.parameters.map((p) => {
                const detail = doc?.parameters[p.name]
                return (
                  <tr key={p.name}>
                    <td>
                      <code>{p.name}</code>
                      <span
                        className={
                          p.required
                            ? 'reference-required'
                            : 'reference-optional'
                        }
                      >
                        {p.required ? translate('必填') : translate('可选')}
                      </span>
                    </td>
                    <td>
                      <code>
                        {detail ? schemaType(detail.schema) : 'string'}
                      </code>
                    </td>
                    <td>
                      {detail?.description || p.description}
                      {detail?.schema.default !== undefined && (
                        <span className='reference-parameter-detail'>
                          {translate('默认：')}
                          <code>{String(detail.schema.default)}</code>
                        </span>
                      )}
                      {detail?.schema.enum && (
                        <span className='reference-parameter-detail'>
                          {translate('取值：')}
                          <code>{detail.schema.enum.join(' · ')}</code>
                        </span>
                      )}
                      {detail?.schema.minimum !== undefined && (
                        <span className='reference-parameter-detail'>
                          {translate('范围：')}
                          {detail.schema.minimum}–{detail.schema.maximum}
                        </span>
                      )}
                      {detail?.schema.maxLength && (
                        <span className='reference-parameter-detail'>
                          {translate('最长 ')}
                          {detail.schema.maxLength}
                          {translate(' 字符')}
                        </span>
                      )}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      ) : (
        <p className='reference-no-params'>
          {translate('无需查询参数，仅需携带 Authorization 请求头。')}
        </p>
      )}
      <h3>{translate('请求示例')}</h3>
      <p>
        {translate(
          '示例从环境变量读取密钥；必填参数中的示例标识需替换为自己的查询结果。下方输入参数后，示例会同步更新。'
        )}
      </p>
      <RequestExample path={examplePath} binary={binary} />
      <h3>{translate('成功响应')}</h3>
      {binary && (
        <p className='reference-callout'>
          {translate('HTTP 200 返回原始')}
          {doc?.content_type === 'image/*'
            ? translate('图片')
            : translate('文件')}
          {translate(
            '字节。读取为 Blob、ArrayBuffer 或字节流，再保存或展示。错误响应仍为 JSON。'
          )}
        </p>
      )}
      {!binary && doc && (
        <>
          <p>{translate('以下为演示数据，用于说明响应结构。')}</p>
          <div className='code-example reference-response-example'>
            <div>
              <span>200 · application/json</span>
              <CopyButton
                value={JSON.stringify(doc.response_example, null, 2)}
              />
            </div>
            <CodeBlock
              code={JSON.stringify(doc.response_example, null, 2)}
              language='json'
            />
          </div>
          <h3>{translate('返回字段')}</h3>
          <p>
            {translate(
              '字段是否为空取决于学校数据；结构化对象可展开查看。未列为固定枚举的文字状态请按原值保留。'
            )}
          </p>
          <SchemaFields schema={doc.response_schema} />
        </>
      )}
      {!binary && !doc && (
        <p>{translate('JSON 响应包含 success、data 和可选 meta。')}</p>
      )}
      <p>
        <Link
          to='/api-reference'
          activeOptions={{ includeHash: true }}
          hash='responses'
        >
          {translate('通用响应、分页与响应头 ')}
          <ArrowRight size={13} />
        </Link>{' '}
        ·{' '}
        <Link
          to='/api-reference'
          activeOptions={{ includeHash: true }}
          hash='limits'
        >
          {translate('错误码与重试说明 ')}
          <ArrowRight size={13} />
        </Link>
      </p>
      <h3>{translate('在线调试')}</h3>
      <p>
        {translate(
          '使用真实密钥发起请求，会计入实际调用额度。密钥仅保留在当前页面内存中，切换接口或刷新后清空。'
        )}
      </p>
      <RequestExplorer
        endpoint={e}
        path={path}
        values={values}
        setValues={setValues}
      />
      {!!doc?.related.length && (
        <>
          <h3>{translate('相关接口')}</h3>
          <div className='reference-related'>
            {doc.related.map((path) => (
              <Link
                key={path}
                to='/api-reference'
                activeOptions={{ includeHash: true }}
                hash={`api${path}`}
              >
                <span>
                  {endpoints.find((item) => item.path === path)?.name || path}
                </span>
                <ArrowRight size={15} />
              </Link>
            ))}
          </div>
        </>
      )}
    </>
  )
}

function RequestExplorer({
  endpoint: e,
  path,
  values,
  setValues,
}: {
  endpoint: Endpoint
  path: string
  values: Record<string, string>
  setValues: (values: Record<string, string>) => void
}) {
  useLocale()
  const [key, setKey] = useState('')
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const [result, setResult] = useState<{
    status: number
    text: string
    json: boolean
    elapsed: number
    remaining: string | null
    retry: string | null
    requestId: string | null
  } | null>(null)
  async function send() {
    setError('')
    setPending(true)
    setResult(null)
    const started = performance.now()
    try {
      const response = await fetch(path, {
        headers: {
          Authorization: `Bearer ${key.trim()}`,
          'Accept-Language': currentLanguage(),
        },
        credentials: 'omit',
        cache: 'no-store',
        signal: AbortSignal.timeout(90000),
      })
      const json = !!response.headers
        .get('content-type')
        ?.includes('application/json')
      let text: string
      if (json) {
        const raw = await response.text()
        try {
          text = JSON.stringify(JSON.parse(raw), null, 2)
        } catch {
          text = raw
        }
      } else {
        const body = await response.arrayBuffer()
        text = translate(
          '{{v0}} · {{v1}} 字节\n二进制响应已接收；保存文件请使用上方代码示例。',
          {
            v0:
              response.headers.get('content-type') ||
              'application/octet-stream',
            v1: body.byteLength.toLocaleString(),
          }
        )
      }
      setResult({
        status: response.status,
        text:
          text.slice(0, 100000) +
          (text.length > 100000 ? translate('\n… 响应展示已截断') : ''),
        json,
        elapsed: Math.round(performance.now() - started),
        remaining: response.headers.get('X-Quota-Remaining'),
        retry: response.headers.get('Retry-After'),
        requestId: response.headers.get('X-Request-Id'),
      })
    } catch (err) {
      setError(
        err instanceof Error && err.name === 'TimeoutError'
          ? translate(
              '请求超时，请稍后在调用日志中确认结果，避免立即重复请求。'
            )
          : translate('请求未完成，请检查网络连接后重试。')
      )
    } finally {
      setPending(false)
    }
  }
  return (
    <>
      <form
        className='editor-form explorer-form'
        onSubmit={(event) => {
          event.preventDefault()
          void send()
        }}
      >
        <label>
          {translate('应用密钥')}
          <Input
            type='password'
            autoComplete='off'
            spellCheck={false}
            required
            placeholder='lc_…'
            value={key}
            onChange={(event) => setKey(event.target.value)}
          />
        </label>
        <div className='form-grid'>
          {e.parameters.map((param) => (
            <label key={param.name}>
              {param.name}
              {param.required ? ' *' : ''}
              <Input
                required={param.required}
                value={values[param.name] || ''}
                placeholder={String(
                  e.documentation?.parameters[param.name]?.example ??
                    param.example ??
                    param.description
                )}
                onChange={(event) =>
                  setValues({ ...values, [param.name]: event.target.value })
                }
              />
            </label>
          ))}
        </div>
        <Button
          type='submit'
          disabled={
            pending ||
            !key.trim() ||
            e.status === 'disabled' ||
            e.status === 'maintenance'
          }
        >
          <Send size={15} />
          {pending ? translate('请求中…') : translate('发送请求')}
        </Button>
        {(e.status === 'disabled' || e.status === 'maintenance') && (
          <p className='quiet-text'>
            {translate('接口当前不可用，请等待恢复后调试。')}
          </p>
        )}
      </form>
      <div aria-live='polite' aria-busy={pending}>
        {error && (
          <p role='alert' className='form-error'>
            {error}
          </p>
        )}
        {result && (
          <div className='response-block'>
            <div>
              <strong>HTTP {result.status}</strong>
              <span>{result.elapsed} ms</span>
              {result.remaining !== null && (
                <span>
                  {translate('剩余额度 ')}
                  {result.remaining}
                </span>
              )}
              {result.retry && <span>Retry-After: {result.retry}s</span>}
            </div>
            {result.requestId && (
              <p className='reference-request-id'>
                {translate('请求标识 ')}
                <code>{result.requestId}</code>
                <CopyButton value={result.requestId} />
              </p>
            )}
            <CodeBlock
              code={result.text}
              language={result.json ? 'json' : 'bash'}
            />
          </div>
        )}
      </div>
    </>
  )
}
