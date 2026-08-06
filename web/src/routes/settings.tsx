// 参照: docs/plans/admin-spa/design.md §4.1/§4.2・§5.3
//
// GET /api/settings で現在の site_name を読み込んで表示。
// PUT /api/settings で更新。
// - 400 (1〜100文字外 = "invalid site_name") -> インラインエラー。
// - 成功時はトースト表示。

import { useEffect, useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { settingsQueryOptions, updateSettings } from '../api/settings'
import { changePassword } from '../api/account'
import { ApiError } from '../api/client'
import { toUserMessage } from '../api/errorMessages'
import { isValidPassword, isValidSiteName } from '../api/validation'

export default function Settings() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const { data, isLoading, isError } = useQuery(settingsQueryOptions())
  const updateMutation = useMutation({ mutationFn: updateSettings })
  const changePasswordMutation = useMutation({ mutationFn: changePassword })

  const [siteName, setSiteName] = useState('')
  const [fieldError, setFieldError] = useState<string | null>(null)
  const [toastMessage, setToastMessage] = useState<string | null>(null)

  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [passwordFieldError, setPasswordFieldError] = useState<string | null>(null)

  useEffect(() => {
    if (data !== undefined) {
      setSiteName(data.site_name)
    }
  }, [data])

  async function handleSubmit(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault()
    setToastMessage(null)

    if (!isValidSiteName(siteName)) {
      setFieldError(toUserMessage('invalid site_name'))
      return
    }
    setFieldError(null)

    try {
      const result = await updateMutation.mutateAsync({ site_name: siteName })
      queryClient.setQueryData(['settings'], result)
      setToastMessage('保存しました。')
    } catch (err) {
      if (err instanceof ApiError) {
        setFieldError(toUserMessage(err.message))
      } else {
        setFieldError('通信エラーが発生しました。')
      }
    }
  }

  async function handlePasswordSubmit(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault()

    if (!isValidPassword(newPassword)) {
      setPasswordFieldError(toUserMessage('invalid new_password'))
      return
    }
    setPasswordFieldError(null)

    try {
      await changePasswordMutation.mutateAsync({
        current_password: currentPassword,
        new_password: newPassword,
      })
      // 成功時は全セッション(このタブ自身のセッションも含む)が失効する
      // ので、必ずログイン画面へ遷移する。キャッシュのclearは行わない —
      // 次に何らかのAPI呼び出しが401を受け取った時点でclient.tsのグローバル
      // ハンドラが行う(このコンポーネントがまだマウントされたままclear()
      // すると、useQuery(settingsQueryOptions())が即座に再フェッチしてしまう)。
      void navigate({ to: '/login' })
    } catch (err) {
      if (err instanceof ApiError) {
        setPasswordFieldError(toUserMessage(err.message))
      } else {
        setPasswordFieldError('通信エラーが発生しました。')
      }
    }
  }

  return (
    <div>
      <h1>設定</h1>

      {toastMessage !== null && <p role="status">{toastMessage}</p>}

      {isLoading && <p>読み込み中...</p>}
      {isError && <p role="alert">設定の取得に失敗しました。</p>}

      {!isLoading && !isError && (
        <form onSubmit={(event) => void handleSubmit(event)}>
          <div>
            <label htmlFor="settings-site-name">サイト名</label>
            <input
              id="settings-site-name"
              type="text"
              value={siteName}
              onChange={(event) => setSiteName(event.target.value)}
            />
            {fieldError !== null && <p role="alert">{fieldError}</p>}
          </div>

          <button type="submit" disabled={updateMutation.isPending}>
            保存
          </button>
        </form>
      )}

      <h2>パスワード変更</h2>
      <p>変更すると、このタブを含む全てのログインセッションが失効します。変更後は再度ログインしてください。</p>
      <form onSubmit={(event) => void handlePasswordSubmit(event)}>
        <div>
          <label htmlFor="settings-current-password">現在のパスワード</label>
          <input
            id="settings-current-password"
            type="password"
            value={currentPassword}
            onChange={(event) => setCurrentPassword(event.target.value)}
          />
        </div>
        <div>
          <label htmlFor="settings-new-password">新しいパスワード</label>
          <input
            id="settings-new-password"
            type="password"
            value={newPassword}
            onChange={(event) => setNewPassword(event.target.value)}
          />
          {passwordFieldError !== null && <p role="alert">{passwordFieldError}</p>}
        </div>

        <button type="submit" disabled={changePasswordMutation.isPending}>
          パスワードを変更
        </button>
      </form>
    </div>
  )
}
