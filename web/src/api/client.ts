// 参照: docs/plans/admin-spa/design.md §4.2/§4.3/§5.2
//
// apiFetch は全APIコールの単一入口。
// - credentials: 'include' を常に固定し、HTTPOnlyクッキーをセッション認証に使う。
// - Content-Type を見て JSON ({"error":"..."}) とプレーンテキスト
//   (AuthMiddleware/CSRFMiddleware由来) の両方のエラーレスポンスを
//   ApiError{status,message} に正規化する。
// - 401 を検知したらグローバルに queryClient.clear() し、
//   configureApiClient で登録された onUnauthorized コールバックを呼ぶ。
//   実際の router.navigate({to:'/admin/login'}) への配線はT5が行う
//   (router シングルトンがまだ存在しないため、ここでは呼び出し口だけ開けておく)。

import type { QueryClient } from '@tanstack/react-query'

export class ApiError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

interface ApiClientConfig {
  queryClient: QueryClient
  onUnauthorized: () => void
}

let clientConfig: ApiClientConfig | null = null

/**
 * グローバルな queryClient 参照と 401 時のコールバックを登録する。
 * T5 がルーターのシングルトンを作った時点で `onUnauthorized: () => router.navigate({to:'/admin/login'})`
 * を渡して配線を完成させる。
 */
export function configureApiClient(config: ApiClientConfig): void {
  clientConfig = config
}

export interface ApiFetchInit {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  headers?: Record<string, string>
  body?: string
  signal?: AbortSignal
  /**
   * POST /api/session (ログイン) は誤ったパスワードでも401を返すが、これは
   * 「セッション切れ」ではないため、グローバルなqueryClient.clear()+
   * onUnauthorizedによるログイン画面への強制遷移を発火させてはならない
   * (ログインフォーム自身がその場でインラインエラーを表示する)。
   * このケースでのみ true を渡し、401時のグローバルハンドラをスキップする。
   */
  skipUnauthorizedRedirect?: boolean
}

async function parseErrorMessage(res: Response): Promise<string> {
  const contentType = res.headers.get('content-type') ?? ''
  if (contentType.includes('application/json')) {
    try {
      const data = (await res.json()) as { error?: unknown }
      if (typeof data.error === 'string' && data.error.length > 0) {
        return data.error
      }
    } catch {
      // JSONとして解釈できない場合はstatusTextにフォールバック
    }
    return res.statusText || `HTTP ${res.status}`
  }

  // AuthMiddleware/CSRFMiddleware由来のプレーンテキスト(http.Error は末尾に改行を付与する)
  const text = await res.text()
  const trimmed = text.trim()
  return trimmed.length > 0 ? trimmed : res.statusText || `HTTP ${res.status}`
}

/**
 * 全APIコールの単一入口。credentials は常に 'include'。
 * 失敗レスポンスは JSON/プレーンテキストいずれの形状でも ApiError に正規化して throw する。
 */
export async function apiFetch<T>(path: string, init: ApiFetchInit = {}): Promise<T> {
  const res = await fetch(path, {
    method: init.method ?? 'GET',
    headers: init.headers,
    body: init.body,
    signal: init.signal,
    credentials: 'include',
  })

  if (!res.ok) {
    const message = await parseErrorMessage(res)
    const error = new ApiError(res.status, message)
    console.error(error)
    if (res.status === 401 && !init.skipUnauthorizedRedirect) {
      clientConfig?.queryClient.clear()
      clientConfig?.onUnauthorized()
    }
    throw error
  }

  // 204 No Content、および 201 空body(POST /api/setup)を空レスポンスとして扱う。
  const text = await res.text()
  if (text.length === 0) {
    return undefined as T
  }
  return JSON.parse(text) as T
}
