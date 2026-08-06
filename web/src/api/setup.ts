// 参照: docs/plans/admin-spa/design.md §4.2 (POST /api/setup)

import { apiFetch } from './client'
import type { SetupRequest } from '../types'

/**
 * POST /api/setup。セットアップトークンは Authorization: Bearer ヘッダーで送る。
 * トークンはURLやlocalStorageに保存せず、呼び出し元のフォーム入力欄の値のみを使う。
 * 成功時 201 空body。
 */
export function setup(token: string, payload: SetupRequest): Promise<void> {
  return apiFetch<void>('/api/setup', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(payload),
  })
}
