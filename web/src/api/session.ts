// 参照: docs/plans/admin-spa/design.md §4.2 (POST/DELETE /api/session)

import { apiFetch } from './client'
import type { LoginRequest } from '../types'

/**
 * POST /api/session。成功時 204 + Set-Cookie(クッキーはSPAのJSからは読み書きしない)。
 * 誤ったパスワードでも401を返すが、これは「セッション切れ」ではないため
 * skipUnauthorizedRedirect:true でグローバルな401ハンドラ(強制ログアウト
 * ナビゲーション)を無効化する。呼び出し元(login画面)が自分でインライン
 * エラーを表示する。
 */
export function login(payload: LoginRequest): Promise<void> {
  return apiFetch<void>('/api/session', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
    skipUnauthorizedRedirect: true,
  })
}

/** DELETE /api/session。成功時 204 + クッキー失効。 */
export function logout(): Promise<void> {
  return apiFetch<void>('/api/session', { method: 'DELETE' })
}
