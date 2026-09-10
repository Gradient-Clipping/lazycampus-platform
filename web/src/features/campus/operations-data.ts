import { useQuery } from '@tanstack/react-query'

import { request } from './api'
import type { PublicSite } from './types'

export const statusLabels: Record<string, string> = {
  operational: '正常',
  maintenance: '维护中',
  degraded: '服务降级',
  disabled: '已停用',
}
export function useSite() {
  return useQuery({
    queryKey: ['site'],
    queryFn: () => request<PublicSite>('/site'),
    staleTime: 60000,
  })
}
export function filterQuery(filters: Record<string, string>) {
  return new URLSearchParams(
    Object.entries(filters).filter(([, value]) => value !== '')
  ).toString()
}
