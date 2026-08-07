// 参照: docs/plans/admin-spa/design.md §2 (root.tsx: ルートレイアウト — ヘッダー、ログアウトボタン)
// docs/plans/p1-multi-user-rbac/design.md §5・tasks.md T15
//
// ナビゲーションリンクはここに集約する(design.md策定時点では未着手だった)。
// /login・/setup はまだセッションが存在しない(あるいはセットアップ未完了)
// 画面のため、useCurrentUserのGET /api/accountを呼ぶと確実に401になり、
// 無意味なリクエストの上にclient.tsのグローバル401ハンドラが余計な
// queryClient.clear()を発火させる。そのためこの2画面ではクエリ自体を
// enabled:falseで止める。

import { Link, Outlet, useLocation, useRouter } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { logout } from '../api/session'
import { useCurrentUser } from '../hooks/useCurrentUser'

const UNAUTHENTICATED_PATHS = ['/login', '/setup']

export default function Root() {
  const router = useRouter()
  const queryClient = useQueryClient()
  const location = useLocation()
  const isUnauthenticatedRoute = UNAUTHENTICATED_PATHS.includes(location.pathname)
  const { hasCapability } = useCurrentUser({ enabled: !isUnauthenticatedRoute })

  async function handleLogout(): Promise<void> {
    await logout()
    queryClient.clear()
    void router.navigate({ to: '/login' })
  }

  return (
    <div>
      <header>
        <span>xordpress admin</span>
        {!isUnauthenticatedRoute && (
          <nav>
            <Link to="/posts">投稿</Link>
            <Link to="/settings">設定</Link>
            <Link to="/account/security">2段階認証</Link>
            <Link to="/account/sessions">セッション</Link>
            {hasCapability('users:manage') && <Link to="/users">ユーザー管理</Link>}
          </nav>
        )}
        <button type="button" onClick={() => void handleLogout()}>
          ログアウト
        </button>
      </header>
      <main>
        <Outlet />
      </main>
    </div>
  )
}
