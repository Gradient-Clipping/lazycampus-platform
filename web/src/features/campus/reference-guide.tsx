import { Link } from '@tanstack/react-router'
import { ArrowRight, KeyRound, Terminal, ShieldCheck } from 'lucide-react'

import { translate, useLocale } from '@/i18n/translate'

import { CodeBlock } from './code-block'
import { RequestExample } from './reference-code'
import { apiOrigin, apiErrors } from './reference-data'
import { useSession } from './session'

export function ReferenceGuide({
  id,
  quickstartScope,
}: {
  id: string
  quickstartScope: string
}) {
  useLocale()
  const { user, limits } = useSession()
  if (id === 'authentication') {
    return (
      <>
        <p className='reference-eyebrow'>{translate('接入指南 / 02')}</p>
        <h2>{translate('鉴权与权限')}</h2>
        <p className='reference-lead'>
          {translate(
            '每个应用使用独立密钥，数据访问范围由所属学校身份和应用权限共同决定。'
          )}
        </p>
        <h3>{translate('请求头')}</h3>
        <CodeBlock code='Authorization: Bearer lc_YOUR_APPLICATION_KEY' />
        <p>
          {translate('所有 ')}
          <code>/v1/</code>
          {translate(
            ' 接口都需要此请求头。不要把密钥放在 URL、查询参数或公开代码中。示例从环境变量'
          )}{' '}
          <code>LAZYCAMPUS_API_KEY</code>
          {translate(' 读取密钥。')}
        </p>
        <h3>{translate('创建与保管密钥')}</h3>
        <ol className='reference-steps'>
          <li>
            {translate('在')}
            <Link to='/apps'>{translate('应用管理')}</Link>
            {translate('创建应用，只勾选需要的权限。')}
          </li>
          <li>
            {translate(
              '完整密钥只在创建或轮换时显示一次，保存到自己的服务端环境变量或本地脚本环境。'
            )}
          </li>
          <li>
            {translate(
              '多人协作或公开网页应通过自己的后端调用，避免将密钥嵌入网页或小程序包。'
            )}
          </li>
          <li>
            {translate('密钥泄露时立即轮换或停用应用；轮换不会重置已用额度。')}
          </li>
        </ol>
        <h3>{translate('权限与数据归属')}</h3>
        <p>
          {translate('每个接口标有所需权限，例如 ')}
          <code>timetable:read</code>
          {translate(
            '。调用密钥所属应用必须包含该权限。权限或接口状态可能由管理员调整，以当前文档为准。'
          )}
        </p>
        <p>
          {translate(
            '应用始终读取所属学校用户的数据，不接受指定其他学号或用户 ID。控制台使用 Keycloak SSO 登录；浏览器登录会话不能替代应用密钥。'
          )}
        </p>
        <h3>{translate('IP 白名单与有效期')}</h3>
        <p>
          {translate(
            '如设置了 IP 白名单，请填写调用服务器的公网出口 IP 或 CIDR。长期应用仅取消到期时间，仍受权限、额度和限流约束。账号或应用停用、授权撤销后，调用会被拒绝。'
          )}
        </p>
      </>
    )
  }
  if (id === 'responses') {
    return (
      <>
        <p className='reference-eyebrow'>{translate('接入指南 / 03')}</p>
        <h2>{translate('响应与分页')}</h2>
        <p className='reference-lead'>
          {translate(
            '先判断 HTTP 状态与 Content-Type，再解析响应体。成功不一定意味着数据刚刚更新。'
          )}
        </p>
        <h3>{translate('JSON 成功响应')}</h3>
        <CodeBlock
          language='json'
          code={JSON.stringify(
            {
              success: true,
              data: {
                items: [],
                pagination: { page: 1, pageSize: 20, total: 0, totalPages: 0 },
              },
              meta: {
                cached: true,
                fetchedAt: '2026-09-11T08:00:00.000Z',
                refreshing: false,
              },
            },
            null,
            2
          )}
        />
        <p>
          {translate('上例用于说明通用结构。')}
          <code>data</code>{' '}
          {translate('以各接口的字段表为准；只有分页接口包含 ')}
          <code>pagination</code>
          {translate('，')}
          <code>meta</code>
          {translate(' 及其中字段按需返回。')}
        </p>
        <div className='reference-table-wrap'>
          <table>
            <thead>
              <tr>
                <th>{translate('字段')}</th>
                <th>{translate('含义')}</th>
              </tr>
            </thead>
            <tbody>
              {[
                ['success', translate('成功为 true；错误为 false。')],
                [
                  'data',
                  translate(
                    '本次查询结果；空数组、null 或统计不足也可以是正常结果。'
                  ),
                ],
                ['meta.cached', translate('是否读取已有数据。')],
                [
                  'meta.fetchedAt',
                  translate('数据版本时间，可与本地记录比较新旧。'),
                ],
                [
                  'meta.refreshing',
                  translate('返回已有数据，同时正在后台更新。'),
                ],
                [
                  'meta.stale',
                  translate('更新失败但仍有可用旧数据；显示更新时间。'),
                ],
                [
                  'meta.deleted',
                  translate(
                    '资源已删除；收到更高版本的删除状态时，应清除对应本地旧数据。'
                  ),
                ],
              ].map(([field, description]) => (
                <tr key={field}>
                  <td>
                    <code>{field}</code>
                  </td>
                  <td>{description}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <h3>{translate('分页与查询编码')}</h3>
        <p>
          <code>page</code>
          {translate(' 从 1 开始，')}
          <code>page</code>
          {translate(' 和')} <code>pageSize</code>
          {translate(' 最大均为 100。每个接口的默认页大小见参数表。按')}{' '}
          <code>pagination.totalPages</code>{' '}
          {translate('停止翻页；消息和通知提供的是有限历史列表。')}
        </p>
        <p>
          {translate('查询参数使用 UTF-8 和标准 URL 编码，建议使用')}{' '}
          <code>URLSearchParams</code>
          {translate(
            '。每个参数只传一次；空教室的多个楼栋和节次使用英文逗号分隔。未知参数返回 400。开放平台不提供 '
          )}
          <code>refresh</code>
          {translate('、')}
          <code>automatic</code>
          {translate(' 或')} <code>waitForSync</code>
          {translate(' 参数。')}
        </p>
        <h3>{translate('图片、附件与时间')}</h3>
        <p>
          {translate('校历原图与通知附件返回二进制，不能调用 ')}
          <code>response.json()</code>{' '}
          {translate(
            '解析；失败时则返回 JSON 错误。单次响应最多 16 MiB。附件名取自通知正文，服务端不保证返回原始文件名。'
          )}
        </p>
        <p>
          {translate('纯日期使用 ')}
          <code>YYYY-MM-DD</code>
          {translate('，周期时间使用学校时区')} <code>Asia/Shanghai</code>
          {translate('。带 ')}
          <code>Z</code>
          {translate(' 或时区偏移的 ISO 8601 时间应转换至用户所在时区显示。')}
        </p>
        <h3>{translate('响应头')}</h3>
        <div className='reference-table-wrap'>
          <table>
            <thead>
              <tr>
                <th>{translate('响应头')}</th>
                <th>{translate('说明')}</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>
                  <code>X-Request-Id</code>
                </td>
                <td>
                  {translate('平台请求标识；反馈问题和查询调用日志时提供。')}
                </td>
              </tr>
              <tr>
                <td>
                  <code>X-Quota-Remaining</code>
                </td>
                <td>
                  {translate(
                    '完成额度预留后返回；账户当日、应用当日、接口当日与应用剩余总额度中的最小值。'
                  )}
                </td>
              </tr>
              <tr>
                <td>
                  <code>RateLimit-Limit</code>
                </td>
                <td>
                  {translate(
                    '进入限流检查后返回；账户和应用每分钟上限的较小值。接口限制、冷却及并发仍单独生效。'
                  )}
                </td>
              </tr>
              <tr>
                <td>
                  <code>Retry-After</code>
                </td>
                <td>
                  {translate(
                    '限流后的建议等待秒数；是否需要调整总额度，还需查看应用用量。'
                  )}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </>
    )
  }
  if (id === 'limits') {
    return (
      <>
        <p className='reference-eyebrow'>{translate('接入指南 / 04')}</p>
        <h2>{translate('额度与错误处理')}</h2>
        <p className='reference-lead'>
          {translate(
            '所有接口免费。账户、应用和接口限制同时生效，创建多个应用不会增加账户可用额度。'
          )}
        </p>
        <div className='reference-facts'>
          <div>
            <span>{translate('当前账户频率')}</span>
            <strong>
              {user.rate_limit}
              {translate(' 次 / 分钟')}
            </strong>
          </div>
          <div>
            <span>{translate('当前账户日额度')}</span>
            <strong>
              {user.daily_quota.toLocaleString()}
              {translate(' 次 / 日')}
            </strong>
          </div>
          <div>
            <span>{translate('账户并发上限')}</span>
            <strong>
              {limits?.user_concurrency || 3}
              {translate(' 个请求')}
            </strong>
          </div>
        </div>
        <h3>{translate('额度如何计算')}</h3>
        <ul className='reference-notes'>
          <li>
            {translate(
              '全部应用共享账户的频率和日额度；每个应用还受自身频率、日额度和总额度约束。'
            )}
          </li>
          <li>
            {translate(
              '接口日额度、每分钟限制与冷却按用户计算，同一用户的应用共享。单接口当前值见接口详情。'
            )}
          </li>
          <li>
            {translate(
              '分钟窗口从首次计入请求起持续 60 秒。每日额度在 UTC 00:00（北京时间 08:00）重置。'
            )}
          </li>
          <li>
            {translate(
              '已接纳请求会扣调用额度，包括校园服务返回的错误。鉴权或网关拒绝不扣调用额度，但可能已经占用频率窗口。'
            )}
          </li>
          <li>
            {translate(
              '应用总额度不会每天重置；轮换密钥、延长有效期也不会清空已用次数。'
            )}
          </li>
        </ul>
        <p>
          {translate(
            '应用总额度可设为无限；每日额度、限流、并发和有效期仍然生效。'
          )}
        </p>
        <h3>{translate('错误响应')}</h3>
        <CodeBlock
          language='json'
          code={JSON.stringify(
            {
              success: false,
              error: {
                code: 'RATE_LIMITED',
                message: translate('请求过于频繁，请稍后重试'),
              },
              request_id: 'example-request-id',
            },
            null,
            2
          )}
        />
        <p>
          {translate('程序分支使用 HTTP 状态和 ')}
          <code>error.code</code>
          {translate('，不要匹配中文说明。校园服务透传错误可能使用 ')}
          <code>requestId</code>
          {translate('；排障时优先记录响应头 ')}
          <code>X-Request-Id</code>
          {translate('。')}
        </p>
        <div className='reference-table-wrap'>
          <table>
            <thead>
              <tr>
                <th>HTTP</th>
                <th>{translate('常见错误码')}</th>
                <th>{translate('处理方式')}</th>
              </tr>
            </thead>
            <tbody>
              {apiErrors.map(([status, code, action]) => (
                <tr key={code}>
                  <td>{status}</td>
                  <td>
                    <code>{code}</code>
                  </td>
                  <td>{translate(action)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p>
          {translate(
            '校园服务可能返回其他业务错误码；保留原始错误信息与请求标识，在'
          )}
          <Link to='/logs'>{translate('调用日志')}</Link>
          {translate('中查看拒绝原因。')}
        </p>
        <h3>{translate('重试建议')}</h3>
        <p>
          {translate(
            '400、401、403、404 先修正参数或权限，再发起请求。429 至少等待'
          )}{' '}
          <code>Retry-After</code>
          {translate(
            '；总额度用尽时停止重试。502、503 可逐次增加等待时间并加入少量随机延迟，建议最多重试 2 次；每次接纳都会计入额度。'
          )}
        </p>
        <p>
          {translate(
            '校园查询可能耗时较长，示例使用 90 秒客户端超时。客户端超时后服务端可能仍在处理，请勿立即并发重发。维护信息见'
          )}
          <Link to='/status'>{translate('接口状态')}</Link>
          {translate('。')}
        </p>
      </>
    )
  }
  return (
    <>
      <p className='reference-eyebrow'>LAZY CAMPUS / OPEN API v1</p>
      <h2>{translate('把校园数据接入你的应用')}</h2>
      <p className='reference-lead'>
        {translate(
          '通过标准 HTTP 接口，在自己的服务或脚本中读取课表、成绩、考试与校园信息。'
        )}
      </p>
      <div className='reference-base'>
        <span>BASE URL</span>
        <code>{apiOrigin}/v1</code>
      </div>
      <div className='reference-quicksteps'>
        <div>
          <KeyRound size={19} />
          <strong>{translate('01 创建应用')}</strong>
          <p>
            {translate('学校身份登录后，在应用管理选择需要的权限并保存密钥。')}
          </p>
          <Link to='/apps'>
            {translate('应用管理 ')}
            <ArrowRight size={14} />
          </Link>
        </div>
        <div>
          <Terminal size={19} />
          <strong>{translate('02 发起请求')}</strong>
          <p>
            {translate('将密钥放入环境变量，通过 Authorization 请求头发送。')}
          </p>
          <Link
            to='/api-reference'
            activeOptions={{ includeHash: true }}
            hash='authentication'
          >
            {translate('鉴权说明 ')}
            <ArrowRight size={14} />
          </Link>
        </div>
        <div>
          <ShieldCheck size={19} />
          <strong>{translate('03 检查结果')}</strong>
          <p>
            {translate('读取响应与剩余额度；出错时用请求标识定位调用记录。')}
          </p>
          <Link to='/logs'>
            {translate('调用日志 ')}
            <ArrowRight size={14} />
          </Link>
        </div>
      </div>
      <h3>{translate('第一条请求')}</h3>
      <p>
        {translate('创建拥有 ')}
        <code>{quickstartScope}</code>
        {translate(' 权限的应用，在运行环境中设置')}{' '}
        <code>LAZYCAMPUS_API_KEY</code>
        {translate(
          '，执行下面的示例。Node.js 示例使用支持内置 fetch 的运行时，Python 示例仅使用标准库。'
        )}
      </p>
      <RequestExample path='/v1/content/feed' />
      <p>
        {translate('没有对本人发布的校园应用内容时，返回空列表属于正常情况：')}
      </p>
      <CodeBlock
        language='json'
        code={JSON.stringify(
          {
            success: true,
            data: {
              items: [],
              announcements: [],
              notifications: [],
              unreadCount: 0,
            },
          },
          null,
          2
        )}
      />
      <h3>{translate('常见接入流程')}</h3>
      <div className='reference-workflows'>
        <Link
          to='/api-reference'
          activeOptions={{ includeHash: true }}
          hash='api/teaching/timetable'
        >
          <strong>{translate('课表与考试')}</strong>
          <span>{translate('获取可用学期 → 选择学期 → 读取安排')}</span>
          <ArrowRight size={16} />
        </Link>
        <Link
          to='/api-reference'
          activeOptions={{ includeHash: true }}
          hash='api/teaching/grades'
        >
          <strong>{translate('成绩与分布')}</strong>
          <span>{translate('查询本人成绩 → 取课程标识 → 查询匿名统计')}</span>
          <ArrowRight size={16} />
        </Link>
        <Link
          to='/api-reference'
          activeOptions={{ includeHash: true }}
          hash='api/teaching/rooms/options'
        >
          <strong>{translate('查询空教室')}</strong>
          <span>{translate('读取校区、楼栋和节次 → 提交查询条件')}</span>
          <ArrowRight size={16} />
        </Link>
        <Link
          to='/api-reference'
          activeOptions={{ includeHash: true }}
          hash='api/teaching/notices'
        >
          <strong>{translate('通知与附件')}</strong>
          <span>{translate('获取通知列表 → 读取正文 → 下载附件')}</span>
          <ArrowRight size={16} />
        </Link>
      </div>
      <p className='reference-callout'>
        {translate(
          '接口示例使用演示数据。学期、课程、楼栋和通知标识，请从自己的查询结果中获取。所有接口只读，密钥仅能访问所属学校用户的数据。'
        )}
      </p>
    </>
  )
}
