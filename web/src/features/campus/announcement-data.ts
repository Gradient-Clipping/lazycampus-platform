import { useQuery } from '@tanstack/react-query'

import { request } from './api'

export interface Announcement {
  id: number
  title: string
  content: string
  created_at: string
}
export function useAnnouncements() {
  return useQuery({
    queryKey: ['announcements'],
    queryFn: () => request<{ items: Announcement[] }>('/announcements'),
    refetchInterval: 60000,
  })
}
