// 参照: docs/plans/p1-multi-user-rbac/design.md §5・tasks.md T16
//
// GET /api/account/sessions の一覧表示。DELETE /api/account/sessions/{id}
// で個別失効(現在のセッション自身も含む)。現在のセッションは視覚的に
// マーキングする。現在のセッションを失効させた場合、直後の一覧再取得が
// 401を受け取り、client.tsのグローバル401ハンドラ(queryClient.clear()+
// ログイン画面への遷移)がそのまま処理するため、ここでの特別分岐は不要。

import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { deleteSessionMutationOptions, sessionsQueryOptions } from '../api/sessions'
import type { Session } from '../api/sessions'

function formatDateTime(unixSeconds: number): string {
  return new Date(unixSeconds * 1000).toLocaleString()
}

export default function AccountSessions() {
  const queryClient = useQueryClient()
  const { data, isLoading, isError } = useQuery(sessionsQueryOptions())
  const deleteMutation = useMutation(deleteSessionMutationOptions())
  const [toastMessage, setToastMessage] = useState<string | null>(null)
  const [revokingID, setRevokingID] = useState<string | null>(null)

  async function handleRevoke(session: Session): Promise<void> {
    setToastMessage(null)
    setRevokingID(session.id)
    try {
      await deleteMutation.mutateAsync(session.id)
      await queryClient.invalidateQueries({ queryKey: ['account', 'sessions'] })
    } catch {
      setToastMessage('失効に失敗しました。')
    } finally {
      setRevokingID(null)
    }
  }

  const sessions = data ?? []

  return (
    <div>
      <h1>ログインセッション</h1>

      {toastMessage !== null && <p role="status">{toastMessage}</p>}

      {isLoading && <p>読み込み中...</p>}
      {isError && <p role="alert">セッション一覧の取得に失敗しました。</p>}

      {!isLoading && !isError && (
        <table>
          <thead>
            <tr>
              <th>デバイス/ブラウザ</th>
              <th>IPアドレス</th>
              <th>ログイン日時</th>
              <th>有効期限</th>
              <th></th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {sessions.map((session) => (
              <tr key={session.id}>
                <td>{session.user_agent !== '' ? session.user_agent : '不明'}</td>
                <td>{session.created_ip !== '' ? session.created_ip : '不明'}</td>
                <td>{formatDateTime(session.created_at)}</td>
                <td>{formatDateTime(session.expires_at)}</td>
                <td>{session.current && <span>このセッション</span>}</td>
                <td>
                  <button
                    type="button"
                    onClick={() => void handleRevoke(session)}
                    disabled={revokingID === session.id}
                  >
                    失効
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}
