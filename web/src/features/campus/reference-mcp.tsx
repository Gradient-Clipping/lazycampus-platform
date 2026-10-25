import { Link } from '@tanstack/react-router'

import { translate } from '@/i18n/translate'

import { CodeBlock } from './code-block'
import { ReferenceHeading } from './reference-heading'

export function McpGuide() {
  return (
    <>
      <p className='reference-eyebrow'>{translate('接入指南 / 05')}</p>
      <ReferenceHeading>{translate('MCP 接入')}</ReferenceHeading>
      <p className='reference-lead'>
        {translate('将 Lazy Campus 连接到 AI 客户端，用对话查询校园数据。')}
      </p>
      <h3>{translate('接入步骤')}</h3>
      <ol className='reference-steps'>
        <li>
          <Link to='/apps'>{translate('打开应用管理')}</Link>
          {translate('，创建应用，勾选需要的权限，并复制应用密钥。')}
        </li>
        <li>
          {translate(
            '在 AI 客户端添加 MCP 服务，连接方式选择 Streamable HTTP，按下方示例填写地址和请求头。'
          )}
        </li>
        <li>
          {translate(
            '将 APPLICATION_KEY 替换为应用密钥，保存配置并刷新工具列表，即可开始使用。'
          )}
        </li>
      </ol>
      <CodeBlock
        language='json'
        code={JSON.stringify(
          {
            mcpServers: {
              lazycampus: {
                url: 'https://platform.lazycampus.com/mcp',
                headers: { Authorization: 'Bearer APPLICATION_KEY' },
              },
            },
          },
          null,
          2
        )}
      />
      <p>{translate('客户端需支持自定义请求头；配置格式以客户端说明为准。')}</p>
      <h3>{translate('可以这样提问')}</h3>
      <ul>
        {[
          '查看我本周的课表和考试安排。',
          '查一下我的成绩。',
          '查询宿舍电费余额和洗衣机状态。',
          '查看已绑定伙伴的课表。',
          '帮我开具在读证明。',
        ].map((example) => (
          <li key={example}>{translate(example)}</li>
        ))}
      </ul>
      <h3>{translate('使用提示')}</h3>
      <ul>
        <li>
          {translate(
            '可用工具由应用权限决定；伙伴和宿舍信息以小程序中的绑定为准。'
          )}
        </li>
        <li>
          {translate(
            '与开放 API 共用调用额度，默认每天 1,500 次、每分钟 30 次。'
          )}
        </li>
        <li>
          {translate(
            '每天首次调用会在后台更新成绩、课表和考试，首次结果可能来自缓存。'
          )}
        </li>
        <li>
          {translate(
            '开具证明需勾选“证明开具”权限。PDF、图片和附件通过对应 API 下载，详见'
          )}
          <Link to='/api-reference' hash='api/campus-documents/files/{id}'>
            {translate('下载证明')}
          </Link>
          {translate('等接口说明。')}
        </li>
      </ul>
    </>
  )
}
