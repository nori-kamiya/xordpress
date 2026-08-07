// 参照: docs/plans/p1-multi-user-rbac/design.md §5 (GET/DELETE /api/account/sessions)

import { mutationOptions, queryOptions } from '@tanstack/react-query'
import { apiFetch } from './client'

export interface Session {
  id: string
  created_at: number
  expires_at: number
  user_agent: string
  created_ip: string
  current: boolean
}

export function listSessions(): Promise<Session[]> {
  return apiFetch<Session[]>('/api/account/sessions')
}

export const sessionsQueryOptions = () =>
  queryOptions({
    queryKey: ['account', 'sessions'] as const,
    queryFn: listSessions,
  })

/** DELETE /api/account/sessions/{id}。成功時 204(対象なしでも204、冪等)。 */
export function deleteSession(id: string): Promise<void> {
  return apiFetch<void>(`/api/account/sessions/${id}`, {
    method: 'DELETE',
  })
}

export const deleteSessionMutationOptions = () =>
  mutationOptions({
    mutationKey: ['account', 'sessions', 'delete'] as const,
    mutationFn: deleteSession,
  })
