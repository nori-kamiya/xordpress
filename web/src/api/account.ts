// roadmap-to-wp-parity.md P0-2: PUT /api/account/password.

import { apiFetch } from './client'

export interface ChangePasswordRequest {
  current_password: string
  new_password: string
}

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
