// 参照: docs/plans/admin-spa/design.md §4.2 (POST /api/setup)

import { apiFetch } from './client'
import type { SetupRequest } from '../types'

/**
 * POST /api/setup。セットアップトークンは Authorization: Bearer ヘッダーで送る。
 * トークンはURLやlocalStorageに保存せず、呼び出し元のフォーム入力欄の値のみを使う。
 * 成功時 201 空body。トークン不一致でも401を返すが、これは「セッション切れ」
 * ではないため skipUnauthorizedRedirect:true でグローバルな401ハンドラを
 * 無効化する(login.tsのloginと同じ理由、web/src/api/client.ts参照)。
 */
export function setup(token: string, payload: SetupRequest): Promise<void> {
  return apiFetch<void>('/api/setup', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(payload),
    skipUnauthorizedRedirect: true,
  })
}
