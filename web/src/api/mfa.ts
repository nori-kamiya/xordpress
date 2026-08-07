// 参照: docs/plans/p1-multi-user-rbac/design.md §5・§6.1・tasks.md T17
//
// 自己管理の2FA登録・無効化(すべてCapSelfManage)と、ログイン2段階目の
// コード検証(POST /api/session/mfa、未認証。既存のPOST /api/session
// と同じくskipUnauthorizedRedirectが必要 — 誤ったコードの401は
// 「セッション切れ」ではない)。

import { apiFetch } from './client'

/** POST /api/account/mfa のレスポンス。 */
export interface StartTOTPEnrollmentResponse {
  secret: string
  otpauth_uri: string
}

/** POST /api/account/mfa。body: 現在のパスワード。
 * 既に確認済みの登録がある場合は409(先にDisableTOTPが必要)。 */
export function startTOTPEnrollment(password: string): Promise<StartTOTPEnrollmentResponse> {
  return apiFetch<StartTOTPEnrollmentResponse>('/api/account/mfa', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ password }),
  })
}

/** POST /api/account/mfa/confirm のレスポンス。recovery_codesは平文で
 * 一度だけ返される(サーバーはbcryptハッシュしか保持しない)。 */
export interface ConfirmTOTPEnrollmentResponse {
  recovery_codes: string[]
}

export function confirmTOTPEnrollment(code: string): Promise<ConfirmTOTPEnrollmentResponse> {
  return apiFetch<ConfirmTOTPEnrollmentResponse>('/api/account/mfa/confirm', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ code }),
  })
}

/** DELETE /api/account/mfa。body: 現在のパスワード。成功時204。 */
export function disableTOTP(password: string): Promise<void> {
  return apiFetch<void>('/api/account/mfa', {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ password }),
  })
}

/** POST /api/session が返す2種類のボディの一方。 */
export interface MFARequiredResponse {
  mfa_required: true
  mfa_token: string
}

/**
 * POST /api/session/mfa。成功時204+Set-Cookie。コードが正しくない場合の
 * 401は「セッション切れ」ではないため、login()と同じくグローバルな401
 * ハンドラを無効化する。
 */
export function verifyMFA(mfaToken: string, code: string): Promise<void> {
  return apiFetch<void>('/api/session/mfa', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ mfa_token: mfaToken, code }),
    skipUnauthorizedRedirect: true,
  })
}
