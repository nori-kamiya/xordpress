// 参照: docs/plans/admin-spa/design.md §5.1・§5.2
//
// username/password フォーム。POST /api/session。
// - 204 -> posts系クエリを invalidate -> ?redirect= があればそちらへ、なければ /posts へ遷移。
// - 401 -> インラインで「ユーザー名またはパスワードが違います」。
// - ?setup=done があれば成功バナーを表示(セットアップ完了直後の遷移。design.md §5.1)。
// - redirect は単純な遷移のみで、編集内容の復元は行わない(design.md §5.2)。

import { useState, type FormEvent } from 'react'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { login } from '../api/session'
import { ApiError } from '../api/client'
import { toUserMessage } from '../api/errorMessages'

export default function Login() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const search = useSearch({ from: '/login' })
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [generalError, setGeneralError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault()
    setGeneralError(null)
    setSubmitting(true)
    try {
      await login({ username, password })
      await queryClient.invalidateQueries({ queryKey: ['posts'] })
      if (search.redirect !== undefined) {
        void navigate({ href: search.redirect })
      } else {
        void navigate({ to: '/posts' })
      }
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 401) {
          setGeneralError('ユーザー名またはパスワードが違います。')
        } else {
          setGeneralError(toUserMessage(err.message))
        }
      } else {
        setGeneralError('通信エラーが発生しました。')
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div>
      <h1>ログイン</h1>

      {search.setup === 'done' && (
        <p role="status">セットアップが完了しました。ログインしてください。</p>
      )}

      {generalError !== null && <p role="alert">{generalError}</p>}

      <form onSubmit={(event) => void handleSubmit(event)}>
        <div>
          <label htmlFor="login-username">ユーザー名</label>
          <input
            id="login-username"
            type="text"
            autoComplete="username"
            value={username}
            onChange={(event) => setUsername(event.target.value)}
          />
        </div>

        <div>
          <label htmlFor="login-password">パスワード</label>
          <input
            id="login-password"
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
        </div>

        <button type="submit" disabled={submitting}>
          ログイン
        </button>
      </form>
    </div>
  )
}
