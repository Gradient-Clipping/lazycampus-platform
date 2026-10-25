import { Link } from '@tanstack/react-router'

import { translate } from '@/i18n/translate'

import { CodeBlock } from './code-block'
import { ReferenceHeading } from './reference-heading'

const overview = `query CampusOverview($semester: String) {
  getMyTimetable(semester: $semester) {
    data { semesters { id label } }
    meta { cached fetchedAt refreshing }
    requestId
  }
  getBoundDormLaundry {
    data {
      binding { campus name }
      devices { id name stateName }
    }
    requestId
  }
}`

const operations = [
  [
    '课表、成绩与考试',
    'getMyTimetable · getMySchedule · getMyGrades · getMyClassGradeDistribution · getCoursePassRates · listExamSemesters · getMyExams',
  ],
  [
    '伙伴课表',
    'listMyCompanions · getBoundCompanionTimetable · getBoundCompanionSchedule',
  ],
  [
    '宿舍生活',
    'getBoundDormElectricity · getBoundDormElectricityHistory · getBoundDormLaundry',
  ],
  [
    '校历与空教室',
    'getAcademicCalendar · getRoomQueryOptions · findAvailableClassrooms',
  ],
  [
    '通知与公告',
    'listMyTeachingMessages · listSchoolNotices · getSchoolNotice · listPlatformAnnouncements',
  ],
  [
    '证明开具',
    'listMyCertificates · getCertificateOptions · getMyCertificateIssueState · issueMyCertificate（Mutation）',
  ],
] as const

export function GraphqlGuide() {
  return (
    <>
      <p className='reference-eyebrow'>{translate('接入指南 / 05')}</p>
      <ReferenceHeading>GraphQL</ReferenceHeading>
      <p className='reference-lead'>
        {translate(
          '按需选择返回字段，在一次请求中查询多项校园数据。GraphQL 与 REST、MCP 共用学校身份、应用权限和调用额度。'
        )}
      </p>
      <h3>{translate('请求方式')}</h3>
      <CodeBlock code='POST https://platform.lazycampus.com/graphql' />
      <p>
        {translate('在')}
        <Link to='/apps'>{translate('应用管理')}</Link>
        {translate(
          '创建应用并选择权限，通过 Authorization: Bearer <APPLICATION_KEY> 鉴权。请求体使用 JSON，包含 query、可选的 variables 和 operationName。'
        )}
      </p>
      <CodeBlock
        code={[
          "curl --fail-with-body --silent --show-error 'https://platform.lazycampus.com/graphql'",
          '  -H "Authorization: Bearer $LAZYCAMPUS_API_KEY"',
          "  -H 'Content-Type: application/json'",
          "  -H 'Accept: application/graphql-response+json'",
          `  --data '{"query":"{ listPlatformAnnouncements { data { unreadCount } requestId } }"}'`,
        ].join(' \\\n')}
      />
      <p>
        {translate(
          '此示例需要 content:read 权限。仅支持 POST；不提供订阅或请求数组批处理。'
        )}
      </p>
      <h3>{translate('查询与字段选择')}</h3>
      <CodeBlock language='graphql' code={overview} />
      <p>
        {translate(
          '示例需要 timetable:read 和 laundry:read 权限。省略 semester 时查询默认学期；指定学期时，将列表返回的 id 放入 variables。每个对象都需选择子字段，未选择的字段不会返回。'
        )}
      </p>
      <CodeBlock
        language='json'
        code={JSON.stringify(
          { query: overview, variables: {}, operationName: 'CampusOverview' },
          null,
          2
        )}
      />
      <h3>{translate('可用操作')}</h3>
      <div className='reference-table-wrap'>
        <table>
          <thead>
            <tr>
              <th>{translate('数据')}</th>
              <th>{translate('GraphQL 字段')}</th>
            </tr>
          </thead>
          <tbody>
            {operations.map(([label, names]) => (
              <tr key={label}>
                <td>{translate(label)}</td>
                <td>
                  <code>{names}</code>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p>
        {translate(
          '共 23 项数据查询和 1 项证明开具操作。参数、权限和返回字段以 Schema 为准；宿舍数据仅查询小程序中已绑定的宿舍。PDF、校历图片和通知附件通过对应 REST 接口下载。'
        )}
      </p>
      <h3>{translate('Schema 与自省')}</h3>
      <p>
        {translate(
          '支持标准自省，可在 GraphQL 客户端中加载 Schema、查看字段、参数及类型，并自动补全查询。'
        )}
      </p>
      <CodeBlock
        language='graphql'
        code={`query Discover {
  __schema {
    queryType { name fields { name description } }
    mutationType { name fields { name description } }
  }
}`}
      />
      <p>
        {translate(
          'JSON 标量用于动态字典及数字或文字混合值，直接选择，不添加子字段。'
        )}
      </p>
      <h3>{translate('错误、额度与数据同步')}</h3>
      <p>
        {translate(
          'HTTP 200 仍可能包含部分失败，需同时检查 data 和 errors。失败字段返回 null，errors[].path 指向该字段；extensions 包含 code、httpStatus、requestId 和 retryAfterSeconds。遇到 429，等待指定秒数后再试。'
        )}
      </p>
      <CodeBlock
        language='json'
        code={JSON.stringify(
          {
            data: { getBoundDormLaundry: null },
            errors: [
              {
                message: 'Application scope is required.',
                path: ['getBoundDormLaundry'],
                extensions: {
                  code: 'SCOPE_REQUIRED',
                  httpStatus: 403,
                  requestId: 'example-request-id',
                  retryAfterSeconds: 0,
                },
              },
            ],
          },
          null,
          2
        )}
      />
      <p>
        {translate(
          '每个顶层数据字段按一次接口调用计入额度，别名也分别计入；减少返回字段不会减少调用次数。默认每天 1,500 次、每分钟 30 次，实际以账户设置为准。自省和 apiGuide 不扣数据额度；协议请求另限每 IP 每分钟 120 次。'
        )}
      </p>
      <p>
        {translate(
          '每次请求最多 10 个顶层字段、16 层深度、1,000 个展开后的选择项，请求体最多 32 KiB，响应最多 2 MiB。分页每次最多 100 条；较大结果请缩小查询或使用 REST。'
        )}
      </p>
      <p>
        {translate(
          '每天首次活动会在后台同步成绩、课表和考试，首次结果可能是旧快照。按需选择 meta，并结合 fetchedAt、updatedAt、cached、refreshing 等实际返回字段判断数据时间。'
        )}
      </p>
      <h3>{translate('证明开具')}</h3>
      <p>
        {translate(
          '证明开具使用 Mutation，需要 documents:issue 权限和 confirm: true 参数。id 来自 listMyCertificates，selected 为 getCertificateOptions 返回的选项键值。'
        )}
      </p>
      <CodeBlock
        language='graphql'
        code={`mutation IssueCertificate($id: String!, $selected: JSON) {
  issueMyCertificate(id: $id, selected: $selected, confirm: true) {
    httpStatus
    data { jobId }
    requestId
  }
}`}
      />
      <p>
        {translate(
          'httpStatus 为 202 表示任务已创建，getMyCertificateIssueState 返回进度和文件信息。此操作不保证幂等；重试前可通过状态接口确认任务是否已创建。'
        )}
      </p>
    </>
  )
}
