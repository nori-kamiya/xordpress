// 参照: docs/plans/admin-spa/design.md §5.1 (初回セットアップ→ログインのhappy path)・§7 (秘密情報の扱い)
//
// セットアップトークン・username・password・site_name を入力し POST /api/setup
// (Authorization: Bearer ヘッダー) を叩く。
// - 201 -> /login?setup=done へナビゲート(成功バナーはログイン画面側の責務)。
// - 409 (setup already completed) -> インラインで案内+ログインへのリンク。
// - 400系 -> errorMessages.ts でフィールド単位のエラーとして表示。
// トークンはURLやlocalStorageに保存せず、送信後は必ず入力欄の状態から破棄する。

import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { setup } from '../api/setup'
import { ApiError } from '../api/client'
import { toUserMessage } from '../api/errorMessages'
import { isValidUsername, isValidPassword, isValidSiteName } from '../api/validation'

interface FieldErrors {
  token?: string
  username?: string
  password?: string
  site_name?: string
}

// サーバーの固定エラー文字列 -> どのフィールドに紐づくか。
// 対応表にない文字列(例: "invalid request body")は汎用バナーに回す。
const SERVER_MESSAGE_FIELD: Record<string, keyof FieldErrors> = {
  unauthorized: 'token',
  'invalid username': 'username',
  'invalid password': 'password',
  'invalid site_name': 'site_name',
}

export default function Setup() {
  const navigate = useNavigate()
  const [token, setToken] = useState('')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [siteName, setSiteName] = useState('')
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({})
  const [generalError, setGeneralError] = useState<string | null>(null)
  const [alreadySetUp, setAlreadySetUp] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  function validate(): FieldErrors {
    const errors: FieldErrors = {}
    if (token.length === 0) {
      errors.token = 'セットアップトークンを入力してください。'
    }
    if (!isValidUsername(username)) {
      errors.username = toUserMessage('invalid username')
    }
    if (!isValidPassword(password)) {
      errors.password = toUserMessage('invalid password')
    }
    if (!isValidSiteName(siteName)) {
      errors.site_name = toUserMessage('invalid site_name')
    }
    return errors
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault()
    setGeneralError(null)
    setAlreadySetUp(false)

    const clientErrors = validate()
    if (Object.keys(clientErrors).length > 0) {
      setFieldErrors(clientErrors)
      return
    }
    setFieldErrors({})

    // 送信直前の値を保持し、フォームの token フィールドは即座に空へ戻す
    // (design.md §7: 送信後は入力欄からも破棄する)。
    const submittedToken = token
    setToken('')
    setSubmitting(true)
    try {
      await setup(submittedToken, { username, password, site_name: siteName })
      void navigate({ to: '/login', search: { setup: 'done' } })
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 409) {
          setAlreadySetUp(true)
        } else {
          const field = SERVER_MESSAGE_FIELD[err.message]
          if (field !== undefined) {
            setFieldErrors({ [field]: toUserMessage(err.message) })
          } else {
            setGeneralError(toUserMessage(err.message))
          }
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
      <h1>初回セットアップ</h1>

      {alreadySetUp && (
        <p role="alert">
          セットアップ済みです。ログインしてください。 <Link to="/login">ログインへ</Link>
        </p>
      )}

      {generalError !== null && <p role="alert">{generalError}</p>}

      <form onSubmit={(event) => void handleSubmit(event)}>
        <div>
          <label htmlFor="setup-token">セットアップトークン</label>
          <input
            id="setup-token"
            type="password"
            autoComplete="off"
            value={token}
            onChange={(event) => setToken(event.target.value)}
          />
          {fieldErrors.token !== undefined && <p role="alert">{fieldErrors.token}</p>}
        </div>

        <div>
          <label htmlFor="setup-username">ユーザー名</label>
          <input
            id="setup-username"
            type="text"
            autoComplete="off"
            value={username}
            onChange={(event) => setUsername(event.target.value)}
          />
          {fieldErrors.username !== undefined && <p role="alert">{fieldErrors.username}</p>}
        </div>

        <div>
          <label htmlFor="setup-password">パスワード</label>
          <input
            id="setup-password"
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
          {fieldErrors.password !== undefined && <p role="alert">{fieldErrors.password}</p>}
        </div>

        <div>
          <label htmlFor="setup-site-name">サイト名</label>
          <input
            id="setup-site-name"
            type="text"
            value={siteName}
            onChange={(event) => setSiteName(event.target.value)}
          />
          {fieldErrors.site_name !== undefined && <p role="alert">{fieldErrors.site_name}</p>}
        </div>

        <button type="submit" disabled={submitting}>
          セットアップを完了する
        </button>
      </form>
    </div>
  )
}
