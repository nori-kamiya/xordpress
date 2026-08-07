// 参照: docs/plans/admin-spa/design.md §4.2 (POST/DELETE /api/session)
// docs/plans/p1-multi-user-rbac/design.md §6.1 (2FAログイン2段階フロー)

import { apiFetch } from './client'
import type { MFARequiredResponse } from './mfa'
import type { LoginRequest } from '../types'

/**
 * POST /api/session。成功時 204(void) + Set-Cookie(クッキーはSPAのJSからは
 * 読み書きしない)。確認済みの2FA登録があるアカウントの場合は、まだ
 * セッションを発行せず200 {mfa_required:true, mfa_token}を返す
 * (design.md §6.1)。呼び出し元はmfa_tokenを使ってPOST /api/session/mfa
 * (mfa.ts の verifyMFA)を完了させる必要がある。
 *
 * 誤ったパスワードでも401を返すが、これは「セッション切れ」ではないため
 * skipUnauthorizedRedirect:true でグローバルな401ハンドラ(強制ログアウト
 * ナビゲーション)を無効化する。呼び出し元(login画面)が自分でインライン
 * エラーを表示する。
 */
export function login(payload: LoginRequest): Promise<MFARequiredResponse | undefined> {
  return apiFetch<MFARequiredResponse | undefined>('/api/session', {
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
