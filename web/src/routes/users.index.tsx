// 参照: docs/plans/p1-multi-user-rbac/design.md §5・tasks.md T15
//
// GET /api/users の一覧・POST /api/users での追加・PATCH /api/users/{id}
// でのロール変更・DELETE /api/users/{id}?reassign_to= での削除
// (削除対象が書いた記事の引き継ぎ先を選択させる)。
// 自分自身の行はロール変更・削除ができない(サーバー側もErrSelfTargetで
// 403/409にするが、ここでは操作不能であることを事前にUIへ反映する)。

import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError } from '../api/client'
import { toUserMessage } from '../api/errorMessages'
import { isValidPassword, isValidUsername } from '../api/validation'
import { useCurrentUser } from '../hooks/useCurrentUser'
import {
  createUserMutationOptions,
  deleteUserMutationOptions,
  updateUserMutationOptions,
  usersQueryOptions,
} from '../api/users'
import type { AdminUser, UserRole } from '../api/users'

const ROLE_LABELS: Record<UserRole, string> = {
  administrator: '管理者',
  editor: '編集者',
  author: '投稿者',
  contributor: '寄稿者',
}

const ROLES: UserRole[] = ['administrator', 'editor', 'author', 'contributor']

export default function UsersIndex() {
  const queryClient = useQueryClient()
  const { currentUser } = useCurrentUser()
  const { data, isLoading, isError } = useQuery(usersQueryOptions())
  const createMutation = useMutation(createUserMutationOptions())
  const updateMutation = useMutation(updateUserMutationOptions())
  const deleteMutation = useMutation(deleteUserMutationOptions())

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState<UserRole>('contributor')
  const [createError, setCreateError] = useState<string | null>(null)

  const [toastMessage, setToastMessage] = useState<string | null>(null)
  const [reassignTargets, setReassignTargets] = useState<Record<number, number>>({})
  const [busyUserID, setBusyUserID] = useState<number | null>(null)

  const users = data ?? []

  async function invalidateUsers(): Promise<void> {
    await queryClient.invalidateQueries({ queryKey: ['users'] })
  }

  async function handleCreate(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault()
    setCreateError(null)

    if (!isValidUsername(username)) {
      setCreateError('ユーザー名は3〜32文字の英数字・アンダースコア・ハイフンで入力してください。')
      return
    }
    if (!isValidPassword(password)) {
      setCreateError('パスワードは8文字以上で入力してください。')
      return
    }

    try {
      await createMutation.mutateAsync({ username, password, role })
      setUsername('')
      setPassword('')
      setRole('contributor')
      await invalidateUsers()
    } catch (err) {
      setCreateError(err instanceof ApiError ? toUserMessage(err.message) : '通信エラーが発生しました。')
    }
  }

  async function handleRoleChange(user: AdminUser, newRole: UserRole): Promise<void> {
    setToastMessage(null)
    setBusyUserID(user.id)
    try {
      await updateMutation.mutateAsync({ id: user.id, role: newRole })
      await invalidateUsers()
    } catch (err) {
      setToastMessage(err instanceof ApiError ? toUserMessage(err.message) : '通信エラーが発生しました。')
    } finally {
      setBusyUserID(null)
    }
  }

  async function handleDelete(user: AdminUser): Promise<void> {
    const reassignTo = reassignTargets[user.id]
    if (reassignTo === undefined) {
      setToastMessage('引き継ぎ先のユーザーを選択してください。')
      return
    }

    setToastMessage(null)
    setBusyUserID(user.id)
    try {
      await deleteMutation.mutateAsync({ id: user.id, reassignTo })
      await invalidateUsers()
    } catch (err) {
      setToastMessage(err instanceof ApiError ? toUserMessage(err.message) : '通信エラーが発生しました。')
    } finally {
      setBusyUserID(null)
    }
  }

  return (
    <div>
      <h1>ユーザー管理</h1>

      {toastMessage !== null && <p role="status">{toastMessage}</p>}

      <form onSubmit={(event) => void handleCreate(event)}>
        <h2>ユーザーを追加</h2>

        {createError !== null && <p role="alert">{createError}</p>}

        <div>
          <label htmlFor="users-new-username">ユーザー名</label>
          <input
            id="users-new-username"
            type="text"
            autoComplete="off"
            value={username}
            onChange={(event) => setUsername(event.target.value)}
          />
        </div>

        <div>
          <label htmlFor="users-new-password">パスワード</label>
          <input
            id="users-new-password"
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
        </div>

        <div>
          <label htmlFor="users-new-role">ロール</label>
          <select id="users-new-role" value={role} onChange={(event) => setRole(event.target.value as UserRole)}>
            {ROLES.map((r) => (
              <option key={r} value={r}>
                {ROLE_LABELS[r]}
              </option>
            ))}
          </select>
        </div>

        <button type="submit" disabled={createMutation.isPending}>
          追加
        </button>
      </form>

      {isLoading && <p>読み込み中...</p>}
      {isError && <p role="alert">ユーザー一覧の取得に失敗しました。</p>}

      {!isLoading && !isError && (
        <table>
          <thead>
            <tr>
              <th>ユーザー名</th>
              <th>ロール</th>
              <th>作成日時</th>
              <th>削除</th>
            </tr>
          </thead>
          <tbody>
            {users.map((user) => {
              const isSelf = user.id === currentUser?.id
              return (
                <tr key={user.id}>
                  <td>{user.username}</td>
                  <td>
                    <select
                      aria-label={`${user.username}のロール`}
                      value={user.role}
                      disabled={isSelf || busyUserID === user.id}
                      onChange={(event) => void handleRoleChange(user, event.target.value as UserRole)}
                    >
                      {ROLES.map((r) => (
                        <option key={r} value={r}>
                          {ROLE_LABELS[r]}
                        </option>
                      ))}
                    </select>
                  </td>
                  <td>{new Date(user.created_at * 1000).toLocaleString()}</td>
                  <td>
                    {isSelf ? (
                      <span>自分自身</span>
                    ) : (
                      <>
                        <label htmlFor={`users-reassign-${user.id}`}>記事の引き継ぎ先</label>
                        <select
                          id={`users-reassign-${user.id}`}
                          value={reassignTargets[user.id] ?? ''}
                          onChange={(event) => {
                            const value = event.target.value
                            setReassignTargets((prev) => {
                              if (value === '') {
                                const { [user.id]: _removed, ...rest } = prev
                                return rest
                              }
                              return { ...prev, [user.id]: Number(value) }
                            })
                          }}
                        >
                          <option value="">選択してください</option>
                          {users
                            .filter((candidate) => candidate.id !== user.id)
                            .map((candidate) => (
                              <option key={candidate.id} value={candidate.id}>
                                {candidate.username}
                              </option>
                            ))}
                        </select>
                        <button
                          type="button"
                          onClick={() => void handleDelete(user)}
                          disabled={busyUserID === user.id}
                        >
                          削除
                        </button>
                      </>
                    )}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      )}
    </div>
  )
}
