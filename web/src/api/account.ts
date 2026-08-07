// roadmap-to-wp-parity.md P0-2: PUT /api/account/password.
// docs/plans/p1-multi-user-rbac/design.md §5: GET /api/account。

import { queryOptions } from '@tanstack/react-query'
import { apiFetch } from './client'

export interface ChangePasswordRequest {
  current_password: string
  new_password: string
}

/** GET /api/account のレスポンス。capabilities は internal/auth の
 * Role.CapabilityNames() が返す文字列(例: "users:manage")の配列。 */
export interface CurrentUser {
  id: number
  username: string
  role: string
  capabilities: string[]
}

/** GET /api/account。ナビゲーション表示の出し分けに使う(cosmeticのみ、
 * 実際の認可は常にサーバー側の各エンドポイントが行う)。 */
export function getCurrentUser(): Promise<CurrentUser> {
  return apiFetch<CurrentUser>('/api/account')
}

export const currentUserQueryOptions = () =>
  queryOptions({
    queryKey: ['account', 'current'] as const,
    queryFn: getCurrentUser,
  })

/**
 * PUT /api/account/password。成功時 204(空body)。全セッションが失効する
 * (呼び出し元自身のセッションも含む)ため、呼び出し元は成功後に必ずログイン
 * 画面へ遷移すること。current_passwordが誤っている場合の401は「セッション
 * 切れ」ではないため、client.tsのグローバル401ハンドラを無効化する
 * (login()/setup()と同じ理由)。
 */
export function changePassword(payload: ChangePasswordRequest): Promise<void> {
  return apiFetch<void>('/api/account/password', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
    skipUnauthorizedRedirect: true,
  })
}
