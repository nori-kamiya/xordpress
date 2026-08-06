// 参照: docs/plans/admin-spa/design.md §2 (root.tsx: ルートレイアウト — ヘッダー、ログアウトボタン)

import { Outlet, useRouter } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { logout } from '../api/session'

export default function Root() {
  const router = useRouter()
  const queryClient = useQueryClient()

  async function handleLogout(): Promise<void> {
    await logout()
    queryClient.clear()
    void router.navigate({ to: '/login' })
  }

  return (
    <div>
      <header>
        <span>xordpress admin</span>
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
