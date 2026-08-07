// 参照: docs/plans/p1-multi-user-rbac/design.md §5・tasks.md T15
//
// GET /api/account を叩いて現在のユーザー(role/capabilities)を返す。
// ナビゲーションの出し分け(例: CapUsersManageを持たない場合は
// 「ユーザー管理」リンクを隠す)にのみ使う。実際の認可はサーバー側の
// 各エンドポイントが行うため、ここでの判定はcosmeticでしかない。

import { useQuery } from '@tanstack/react-query'
import { currentUserQueryOptions } from '../api/account'
import type { CurrentUser } from '../api/account'

export function useCurrentUser(options: { enabled?: boolean } = {}): {
  currentUser: CurrentUser | undefined
  isLoading: boolean
  isError: boolean
  hasCapability: (capability: string) => boolean
} {
  const { data, isLoading, isError } = useQuery({
    ...currentUserQueryOptions(),
    enabled: options.enabled ?? true,
  })

  function hasCapability(capability: string): boolean {
    return data?.capabilities.includes(capability) ?? false
  }

  return { currentUser: data, isLoading, isError, hasCapability }
}
