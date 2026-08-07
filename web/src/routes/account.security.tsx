// 参照: docs/plans/p1-multi-user-rbac/design.md §5・§6.1・tasks.md T17
//
// TOTP登録(secret表示+QRなしの手動入力+confirm)・無効化の画面。
// GET /api/account には「2FAが有効かどうか」の情報がなく、design.md
// にもそのためのGETエンドポイントは存在しない(§5参照)。そのため
// この画面は「現在の状態を表示するダッシュボード」ではなく、
// 「これから登録する/今すぐ無効化する」という操作フォームとして
// 構成する: 既に確認済みの登録がある状態でPOST /api/account/mfaを
// 呼ぶと409が返るので、その場合はインラインで「先に無効化してください」
// と案内する(totp.goのStartTOTPEnrollmentのコメント参照)。

import { useState, type FormEvent } from 'react'
import { useMutation } from '@tanstack/react-query'
import { confirmTOTPEnrollment, disableTOTP, startTOTPEnrollment } from '../api/mfa'
import { ApiError } from '../api/client'
import { toUserMessage } from '../api/errorMessages'

type EnrollmentStep =
  | { stage: 'idle' }
  | { stage: 'confirm'; secret: string; otpauthUri: string }
  | { stage: 'recovery-codes'; codes: string[] }

export default function AccountSecurity() {
  const startMutation = useMutation({ mutationFn: startTOTPEnrollment })
  const confirmMutation = useMutation({ mutationFn: confirmTOTPEnrollment })
  const disableMutation = useMutation({ mutationFn: disableTOTP })

  const [step, setStep] = useState<EnrollmentStep>({ stage: 'idle' })

  const [startPassword, setStartPassword] = useState('')
  const [startError, setStartError] = useState<string | null>(null)

  const [code, setCode] = useState('')
  const [confirmError, setConfirmError] = useState<string | null>(null)

  const [disablePassword, setDisablePassword] = useState('')
  const [disableError, setDisableError] = useState<string | null>(null)
  const [disableToast, setDisableToast] = useState<string | null>(null)

  async function handleStart(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault()
    setStartError(null)
    try {
      const result = await startMutation.mutateAsync(startPassword)
      setStartPassword('')
      setStep({ stage: 'confirm', secret: result.secret, otpauthUri: result.otpauth_uri })
    } catch (err) {
      setStartError(err instanceof ApiError ? toUserMessage(err.message) : '通信エラーが発生しました。')
    }
  }

  async function handleConfirm(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault()
    setConfirmError(null)
    try {
      const result = await confirmMutation.mutateAsync(code)
      setCode('')
      setStep({ stage: 'recovery-codes', codes: result.recovery_codes })
    } catch (err) {
      setConfirmError(err instanceof ApiError ? toUserMessage(err.message) : '通信エラーが発生しました。')
    }
  }

  async function handleDisable(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault()
    setDisableError(null)
    setDisableToast(null)
    try {
      await disableMutation.mutateAsync(disablePassword)
      setDisablePassword('')
      setDisableToast('2段階認証を無効化しました。')
    } catch (err) {
      setDisableError(err instanceof ApiError ? toUserMessage(err.message) : '通信エラーが発生しました。')
    }
  }

  return (
    <div>
      <h1>2段階認証</h1>

      {step.stage === 'idle' && (
        <section>
          <h2>2段階認証を有効にする</h2>
          <form onSubmit={(event) => void handleStart(event)}>
            <div>
              <label htmlFor="security-start-password">現在のパスワード</label>
              <input
                id="security-start-password"
                type="password"
                value={startPassword}
                onChange={(event) => setStartPassword(event.target.value)}
              />
            </div>
            {startError !== null && <p role="alert">{startError}</p>}
            <button type="submit" disabled={startMutation.isPending}>
              登録を開始
            </button>
          </form>
        </section>
      )}

      {step.stage === 'confirm' && (
        <section>
          <h2>認証アプリに登録</h2>
          <p>認証アプリ(Google Authenticator、Authy等)に以下のキーを手動で入力してください。</p>
          <p>
            <code>{step.secret}</code>
          </p>
          <p>otpauth URI: {step.otpauthUri}</p>

          <form onSubmit={(event) => void handleConfirm(event)}>
            <div>
              <label htmlFor="security-confirm-code">認証アプリに表示されたコード</label>
              <input
                id="security-confirm-code"
                type="text"
                autoComplete="one-time-code"
                value={code}
                onChange={(event) => setCode(event.target.value)}
              />
            </div>
            {confirmError !== null && <p role="alert">{confirmError}</p>}
            <button type="submit" disabled={confirmMutation.isPending}>
              確認
            </button>
          </form>
        </section>
      )}

      {step.stage === 'recovery-codes' && (
        <section>
          <h2>リカバリコード</h2>
          <p>
            2段階認証が有効になりました。以下のリカバリコードは今だけ表示されます。認証アプリを
            紛失した場合に使用できるので、安全な場所に保管してください。
          </p>
          <ul>
            {step.codes.map((recoveryCode) => (
              <li key={recoveryCode}>
                <code>{recoveryCode}</code>
              </li>
            ))}
          </ul>
          <button type="button" onClick={() => setStep({ stage: 'idle' })}>
            完了
          </button>
        </section>
      )}

      <section>
        <h2>2段階認証を無効にする</h2>
        {disableToast !== null && <p role="status">{disableToast}</p>}
        <form onSubmit={(event) => void handleDisable(event)}>
          <div>
            <label htmlFor="security-disable-password">現在のパスワード</label>
            <input
              id="security-disable-password"
              type="password"
              value={disablePassword}
              onChange={(event) => setDisablePassword(event.target.value)}
            />
          </div>
          {disableError !== null && <p role="alert">{disableError}</p>}
          <button type="submit" disabled={disableMutation.isPending}>
            無効化
          </button>
        </form>
      </section>
    </div>
  )
}
