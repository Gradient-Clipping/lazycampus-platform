export interface User {
  id: number
  username: string
  display_name: string
  role: 'user' | 'admin'
  enabled: boolean
  daily_quota: number
  rate_limit: number
  inherit_limits?: boolean
  created_at: string
  last_login_at: string
}
export interface Session {
  user: User
  csrf_token: string
  limits?: PlatformPolicy
}
export interface Token {
  id: number
  name: string
  user_id: number
  key_prefix: string
  scopes: string
  allowed_ips: string
  enabled: boolean
  suspended?: boolean
  revoked_at?: string | null
  admin_reason?: string
  daily_quota: number
  rate_limit: number
  quota: number
  used_quota: number
  expires_at: string | null
  created_at: string
  last_used_at: string | null
}
export interface TokenInput {
  name: string
  scopes: string[]
  allowed_ips: string
  daily_quota: number
  rate_limit: number
  quota: number
  expires_in_days: number
  never_expires: boolean
  enabled: boolean
}
export interface Endpoint {
  path: string
  name: string
  scope: string
  description: string
  cooldown_seconds: number
  status: ServiceStatus | 'disabled'
  message: string
  rate_limit: number
  daily_quota: number
  updated_at: string
  documentation?: {
    group: string
    summary: string
    notes: string[]
    related: string[]
    parameters: Record<
      string,
      {
        description: string
        schema: ReferenceSchema
        example?: string | number | boolean
      }
    >
    content_type: string
    response_schema: ReferenceSchema
    response_example: unknown
  }
  parameters: {
    name: string
    description: string
    required: boolean
    example?: string
  }[]
}
export interface ReferenceSchema {
  type?: string
  description?: string
  format?: string
  nullable?: boolean
  enum?: (string | number | boolean)[]
  default?: string | number | boolean
  minimum?: number
  maximum?: number
  maxLength?: number
  pattern?: string
  properties?: Record<string, ReferenceSchema>
  items?: ReferenceSchema
  additionalProperties?: ReferenceSchema | boolean
  anyOf?: ReferenceSchema[]
}
export interface RequestLog {
  id: number
  user_id: number
  token_id: number
  request_id: string
  endpoint: string
  method: string
  status: number
  admitted: boolean
  user_name: string
  token_name: string
  error_code: string
  error_message: string
  upstream_request_id: string
  client_ip: string
  retry_after: number
  limit_scope?: string
  duration_ms: number
  created_at: string
}
export interface Audit {
  id: number
  actor_id: number
  action: string
  target: string
  details?: string
  created_at: string
}
export interface Page<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}
export interface Dashboard {
  total_requests: number
  active_apps: number
  successful_24h: number
  failed_24h: number
  today_requests: number
  daily_quota: number
  rate_limit: number
  series: { day: string; requests: number }[]
  reset_at: string
}
export interface System {
  revision: string
  database: boolean
  redis: boolean
  users: number
  tokens: number
  requests: number
  default_daily_quota: number
  default_rate_limit: number
  concurrency_per_user: number
  concurrency_global: number
  log_retention_days: number
  email_available: boolean
}

export interface PlatformPolicy {
  daily_quota: number
  rate_limit: number
  app_quota: number
  app_days: number
  max_apps: number
  burst: number
  user_concurrency: number
  global_concurrency: number
}
export type ServiceStatus = 'operational' | 'maintenance' | 'degraded'
export interface SiteContent {
  name: string
  description: string
  logo_url: string
  help_url: string
  support_email: string
  service_status: ServiceStatus
  service_message: string
  faq: { id?: string; question: string; answer: string }[]
}
export interface PublicSite {
  site: SiteContent
  limits: PlatformPolicy
  endpoints: Pick<Endpoint, 'path' | 'name' | 'status' | 'message'>[]
  email_available: boolean
}
export interface Metrics {
  requests: number
  admitted: number
  errors: number
  limited: number
  rejected: number
  server_errors: number
  avg_ms: number
  max_ms: number
}
export interface Analytics {
  summary: Metrics
  p95_ms: number
  series: (Metrics & { time: string })[]
  groups: Record<string, (Metrics & { key: string; name: string })[]>
  window: { start: string; end: string }
  bucket: 'hour' | 'day'
  group_limit: number
}
export interface NotificationPreference {
  language?: 'zh' | 'en'
  user_id: number
  email: string
  in_app: boolean
  email_enabled: boolean
  quota_enabled: boolean
  quota_percent: number
  errors_enabled: boolean
  error_percent: number
  error_minimum: number
  rate_enabled: boolean
  expiry_enabled: boolean
  expiry_days: number
  updated_at: string
}
export interface PersonalNotification {
  id: number
  kind: string
  title: string
  content: string
  read_at: string | null
  in_app: boolean
  email_status: string
  created_at: string
}
export interface SessionDevice {
  id: string
  user_id: number
  device: string
  ip: string
  created_at: string
  last_active_at: string
  expires_at: string
}
