// バックエンドJSON形状にそのまま対応する型定義。
// 参照: docs/plans/admin-spa/design.md §4.2

export type PostStatus = 'draft' | 'published'

/** GET /api/posts の items[] 要素 */
export interface PostSummary {
  id: number
  slug: string
  title: string
  status: PostStatus
  version: number
  updated_at: number
}

/** POST/GET/PUT /api/posts(/{id}) のフルレスポンス形状。
 * id/author_id は int64、created_at/updated_at/published_at はUnix秒
 * (time.Now().Unix()、カスタムMarshalJSONなし) — internal/content/post.go,
 * internal/adminapi/posts.go の postResponse と一致させる。 */
export interface Post {
  id: number
  slug: string
  title: string
  body: string
  status: PostStatus
  version: number
  author_id: number
  created_at: number
  updated_at: number
  published_at: number | null
}

/** GET /api/posts のレスポンス */
export interface PostListResponse {
  items: PostSummary[]
  total: number
}

/** POST /api/posts, PUT /api/posts/{id} のリクエストボディ */
export interface PostWriteRequest {
  title: string
  slug: string
  body: string
  status: PostStatus
}

/** POST /api/setup のリクエストボディ */
export interface SetupRequest {
  username: string
  password: string
  site_name: string
}

/** POST /api/session のリクエストボディ */
export interface LoginRequest {
  username: string
  password: string
}

/** GET/PUT /api/settings のレスポンス・リクエスト */
export interface SettingsResponse {
  site_name: string
}

export interface SettingsRequest {
  site_name: string
}
