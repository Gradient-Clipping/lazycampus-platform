import { translate } from '@/i18n/translate'

import type { Endpoint, ReferenceSchema } from './types'

export const apiOrigin = 'https://platform.lazycampus.com'
export type ExampleLanguage = 'bash' | 'javascript' | 'python'

export function requestExample(
  path: string,
  binary: boolean,
  language: ExampleLanguage,
  method = 'GET',
  body = '{}'
) {
  const url = `${apiOrigin}${path}`
  if (language === 'bash') {
    const payload = body.replaceAll("'", "'\"'\"'")
    return `curl --fail-with-body --silent --show-error --max-time 90 '${url}' \\\n  -H "Authorization: Bearer $LAZYCAMPUS_API_KEY"${method === 'POST' ? ` \\\n  -X POST -H 'Content-Type: application/json' \\\n  --data-raw '${payload}'` : ''}${binary ? ' \\\n  --output response.bin' : ''}`
  }
  if (language === 'javascript') {
    return `${binary ? "import { writeFile } from 'node:fs/promises'\n\n" : ''}const apiKey = process.env.LAZYCAMPUS_API_KEY
if (!apiKey) throw new Error(${JSON.stringify(translate('请设置 LAZYCAMPUS_API_KEY'))})

const response = await fetch(${JSON.stringify(url)}, {
  ${method === 'POST' ? `method: 'POST',\n  body: ${JSON.stringify(body)},\n  ` : ''}headers: { Authorization: \`Bearer \${apiKey}\`${method === 'POST' ? ", 'Content-Type': 'application/json'" : ''} },
  signal: AbortSignal.timeout(90000),
})
const requestId = response.headers.get('X-Request-Id')
if (!response.ok) {
  const error = await response.json()
  console.error({ status: response.status, requestId,
    retryAfter: response.headers.get('Retry-After'), ...error })
  throw new Error(${JSON.stringify(translate('请求失败，请按错误码处理'))})
}
${binary ? "await writeFile('response.bin', Buffer.from(await response.arrayBuffer()))" : 'const body = await response.json()\nconsole.log(body.data)'}
console.log(${JSON.stringify(translate('剩余额度:'))}, response.headers.get('X-Quota-Remaining'))`
  }
  return `import ${binary ? 'os' : 'json, os'}
from urllib.request import Request, urlopen
from urllib.error import HTTPError

api_key = os.environ['LAZYCAMPUS_API_KEY']
request = Request(
    ${JSON.stringify(url)},
    headers={'Authorization': f'Bearer {api_key}'${method === 'POST' ? ", 'Content-Type': 'application/json'" : ''}},
${method === 'POST' ? `    method='POST', data=${JSON.stringify(body)}.encode('utf-8'),\n` : ''}\
)
try:
    with urlopen(request, timeout=90) as response:
${binary ? "        with open('response.bin', 'wb') as file:\n            file.write(response.read())" : "        body = json.load(response)\n        print(body['data'])"}
        print(${JSON.stringify(translate('剩余额度:'))}, response.headers.get('X-Quota-Remaining'))
except HTTPError as error:
    print('HTTP:', error.code)
    print(${JSON.stringify(translate('请求标识:'))}, error.headers.get('X-Request-Id'))
    print(${JSON.stringify(translate('重试等待秒数:'))}, error.headers.get('Retry-After'))
    print(error.read().decode('utf-8'))
    raise`
}

export function endpointPath(
  endpoint: Endpoint,
  values: Record<string, string>,
  examples = false
) {
  let path = endpoint.path
  const query = new URLSearchParams()
  for (const parameter of endpoint.parameters) {
    const value =
      values[parameter.name] ||
      (examples && parameter.required
        ? String(
            endpoint.documentation?.parameters[parameter.name]?.example ??
              parameter.example ??
              ''
          )
        : '')
    if (path.includes(`{${parameter.name}}`)) {
      if (value) {
        path = path.replace(`{${parameter.name}}`, encodeURIComponent(value))
      }
    } else if (value) {
      query.set(parameter.name, value)
    }
  }
  return `/v1${path}${query.size ? `?${query}` : ''}`
}

export const referenceGuides = [
  { id: 'start', title: '快速开始' },
  { id: 'authentication', title: '鉴权与权限' },
  { id: 'responses', title: '响应与分页' },
  { id: 'limits', title: '额度与错误处理' },
  { id: 'mcp', title: 'MCP 接入' },
] as const

export const apiErrors = [
  [
    '400',
    'INVALID_QUERY',
    '检查必填参数、参数名、类型和范围；同一参数只传一次。',
  ],
  ['401', 'INVALID_API_KEY', '检查 Bearer 格式与密钥；轮换后必须更新调用端。'],
  [
    '401',
    'APP_DISABLED / APP_SUSPENDED / APP_REVOKED / APP_EXPIRED',
    '检查应用启用状态、管理限制或到期时间。停用和到期不是瞬时故障。',
  ],
  [
    '403',
    'SCOPE_REQUIRED / IP_NOT_ALLOWED',
    '在应用设置核对所需权限与调用服务的公网出口 IP。',
  ],
  [
    '403',
    'ACCOUNT_DISABLED / SCHOOL_IDENTITY_REQUIRED',
    '检查账户和学校身份；无学校身份的管理员也不能查询学生数据。',
  ],
  [
    '404',
    'ENDPOINT_NOT_FOUND / NOTICE_NOT_VISIBLE / CALENDAR_NOT_FOUND',
    '核对完整路径；资源标识必须取自本人最新的选项或列表结果。',
  ],
  [
    '404',
    'GRADE_CLASS_DISTRIBUTION_COURSE_NOT_OWNED',
    '从本人成绩记录同时获取 courseId、academicYear 和 term。',
  ],
  [
    '429',
    'RATE_LIMITED / CONCURRENCY_LIMITED',
    '至少等待 Retry-After 指定的秒数，再降低频率或并发。',
  ],
  [
    '429',
    'QUOTA_EXHAUSTED',
    '检查账户当日、应用当日、接口当日和应用总额度；总额度不会每日重置。',
  ],
  [
    '429',
    'ROOM_QUERY_RATE_LIMITED',
    '校园查询服务的独立限制；遵循 Retry-After。',
  ],
  [
    '502',
    'UPSTREAM_UNAVAILABLE / RESPONSE_TOO_LARGE',
    '稍后有限重试；超过 16 MiB 的文件需到学校原通知页面获取。',
  ],
  [
    '503',
    'ENDPOINT_UNAVAILABLE',
    '查看接口状态页，等待维护结束或管理员恢复接口。',
  ],
  [
    '503',
    'POLICY_UNAVAILABLE / LIMITER_UNAVAILABLE / QUOTA_UNAVAILABLE',
    '平台依赖暂不可用；稍后重试，持续失败时附请求标识反馈。',
  ],
]

export function schemaType(schema: ReferenceSchema): string {
  let type = schema.type || 'unknown'
  if (schema.anyOf) type = schema.anyOf.map(schemaType).join(' | ')
  else if (schema.type === 'array') {
    type = `${schema.items ? schemaType(schema.items) : 'unknown'}[]`
  }
  return schema.nullable ? `${type} | null` : type
}
