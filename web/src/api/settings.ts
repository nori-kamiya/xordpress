// 参照: docs/plans/admin-spa/design.md §4.1/§4.2 (GET/PUT /api/settings)

import { queryOptions } from '@tanstack/react-query'
import { apiFetch } from './client'
import type { SettingsRequest, SettingsResponse } from '../types'

export function getSettings(): Promise<SettingsResponse> {
  return apiFetch<SettingsResponse>('/api/settings')
}

export const settingsQueryOptions = () =>
  queryOptions({
    queryKey: ['settings'] as const,
    queryFn: getSettings,
  })

/** PUT /api/settings。成功時 200 { site_name }。 */
export function updateSettings(payload: SettingsRequest): Promise<SettingsResponse> {
  return apiFetch<SettingsResponse>('/api/settings', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}
