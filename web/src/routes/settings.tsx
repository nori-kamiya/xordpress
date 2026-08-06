// 参照: docs/plans/admin-spa/design.md §4.1/§4.2・§5.3
//
// GET /api/settings で現在の site_name を読み込んで表示。
// PUT /api/settings で更新。
// - 400 (1〜100文字外 = "invalid site_name") -> インラインエラー。
// - 成功時はトースト表示。

import { useEffect, useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { settingsQueryOptions, updateSettings } from '../api/settings'
import { ApiError } from '../api/client'
import { toUserMessage } from '../api/errorMessages'
import { isValidSiteName } from '../api/validation'

export default function Settings() {
  const queryClient = useQueryClient()
  const { data, isLoading, isError } = useQuery(settingsQueryOptions())
  const updateMutation = useMutation({ mutationFn: updateSettings })

  const [siteName, setSiteName] = useState('')
  const [fieldError, setFieldError] = useState<string | null>(null)
  const [toastMessage, setToastMessage] = useState<string | null>(null)

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
    </div>
  )
}
