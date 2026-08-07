// 参照: docs/plans/p1-multi-user-rbac/design.md §5・tasks.md T9/T15
//
// /api/users* はすべて CapUsersManage が必要(サーバー側で強制)。
// このAPIクライアント自体はcapabilityチェックを行わない。

import { mutationOptions, queryOptions } from '@tanstack/react-query'
import { apiFetch } from './client'

export type UserRole = 'administrator' | 'editor' | 'author' | 'contributor'

/** GET /api/users の要素、GET/POST/PATCH /api/users/{id} のレスポンス。 */
export interface AdminUser {
  id: number
  username: string
  role: UserRole
  created_at: number
}

export function listUsers(): Promise<AdminUser[]> {
  return apiFetch<AdminUser[]>('/api/users')
}

export const usersQueryOptions = () =>
  queryOptions({
    queryKey: ['users'] as const,
    queryFn: listUsers,
  })

export interface CreateUserRequest {
  username: string
  password: string
  role: UserRole
}

/** POST /api/users。成功時 201。 */
export function createUser(payload: CreateUserRequest): Promise<AdminUser> {
  return apiFetch<AdminUser>('/api/users', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

export const createUserMutationOptions = () =>
  mutationOptions({
    mutationKey: ['users', 'create'] as const,
    mutationFn: createUser,
  })

export interface UpdateUserRequest {
  id: number
  role: UserRole
}

/** PATCH /api/users/{id}。ロール変更のみ(パスワードリセットのUIはT15の
 * スコープ外 — design.md §5の{role}+任意の{password}のうちroleのみ使う)。 */
export function updateUser({ id, role }: UpdateUserRequest): Promise<AdminUser> {
  return apiFetch<AdminUser>(`/api/users/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ role }),
  })
}

export const updateUserMutationOptions = () =>
  mutationOptions({
    mutationKey: ['users', 'update'] as const,
    mutationFn: updateUser,
  })

export interface DeleteUserRequest {
  id: number
  reassignTo: number
}

/** DELETE /api/users/{id}?reassign_to=。成功時 204。 */
export function deleteUser({ id, reassignTo }: DeleteUserRequest): Promise<void> {
  return apiFetch<void>(`/api/users/${id}?reassign_to=${reassignTo}`, {
    method: 'DELETE',
  })
}

export const deleteUserMutationOptions = () =>
  mutationOptions({
    mutationKey: ['users', 'delete'] as const,
    mutationFn: deleteUser,
  })
