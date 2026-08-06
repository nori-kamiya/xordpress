// 参照: docs/plans/admin-spa/design.md §5.3・§7
//
// ロード時 GET /api/posts/{id}(404 -> not-found表示)。
// title/slug/body/status フォーム。version はフォーム状態に保持するが表示しない(hidden state)。
// 保存は PUT + If-Match:"${version}"。
// - 412 (version mismatch) -> 衝突バナー「この投稿は他の場所で更新されています。最新の内容を読み込みますか?」+
//   「最新を読み込む」ボタン。ボタン押下で再GET -> フォーム値・versionをリセット(未保存の編集内容は破棄)。
// - 409 (slug already exists) -> slug欄にインラインエラー。
// - 400系 -> errorMessages.ts でフィールド単位のエラーとして表示、対応表にない文字列は汎用バナーへ。
// 削除ボタン(DELETE + If-Match、成功後 /posts へ遷移)。

import { useEffect, useState, type FormEvent } from 'react'
import { useNavigate, useParams } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { deletePostMutationOptions, postQueryOptions, updatePostMutationOptions } from '../api/posts'
import { ApiError } from '../api/client'
import { toUserMessage } from '../api/errorMessages'
import { isValidTitle, isValidSlug, isValidBody } from '../api/validation'
import type { Post, PostStatus } from '../types'

interface FieldErrors {
  title?: string
  slug?: string
  body?: string
}

// サーバーの固定エラー文字列 -> どのフィールドに紐づくか。
// 対応表にない文字列(例: "invalid request body")は汎用バナーに回す。
const SERVER_MESSAGE_FIELD: Record<string, keyof FieldErrors> = {
  'invalid title': 'title',
  'invalid slug': 'slug',
  'invalid body': 'body',
}

export default function PostsEdit() {
  const { postId } = useParams({ from: '/posts/$postId/edit' })
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  const postQuery = useQuery(postQueryOptions(postId))
  const updateMutation = useMutation(updatePostMutationOptions())
  const deleteMutation = useMutation(deletePostMutationOptions())

  const [initialized, setInitialized] = useState(false)
  const [title, setTitle] = useState('')
  const [slug, setSlug] = useState('')
  const [body, setBody] = useState('')
  const [status, setStatus] = useState<PostStatus>('draft')
  const [version, setVersion] = useState<number>(0)
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({})
  const [generalError, setGeneralError] = useState<string | null>(null)
  const [showConflictBanner, setShowConflictBanner] = useState(false)

  function resetFormFromPost(post: Post): void {
    setTitle(post.title)
    setSlug(post.slug)
    setBody(post.body)
    setStatus(post.status)
    setVersion(post.version)
  }

  useEffect(() => {
    if (postQuery.data !== undefined && !initialized) {
      resetFormFromPost(postQuery.data)
      setInitialized(true)
    }
  }, [postQuery.data, initialized])

  async function handleReloadLatest(): Promise<void> {
    const result = await postQuery.refetch()
    if (result.data !== undefined) {
      resetFormFromPost(result.data)
      setFieldErrors({})
      setGeneralError(null)
    }
    setShowConflictBanner(false)
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault()
    setGeneralError(null)

    const clientErrors: FieldErrors = {}
    if (!isValidTitle(title)) {
      clientErrors.title = toUserMessage('invalid title')
    }
    if (!isValidSlug(slug)) {
      clientErrors.slug = toUserMessage('invalid slug')
    }
    if (!isValidBody(body)) {
      clientErrors.body = toUserMessage('invalid body')
    }
    if (Object.keys(clientErrors).length > 0) {
      setFieldErrors(clientErrors)
      return
    }
    setFieldErrors({})

    try {
      const updated = await updateMutation.mutateAsync({
        id: postId,
        version,
        payload: { title, slug, body, status },
      })
      setVersion(updated.version)
      await queryClient.invalidateQueries({ queryKey: ['posts'] })
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 412) {
          setShowConflictBanner(true)
        } else if (err.status === 409) {
          setFieldErrors({ slug: toUserMessage(err.message) })
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
    }
  }

  async function handleDelete(): Promise<void> {
    setGeneralError(null)
    try {
      await deleteMutation.mutateAsync({ id: postId, version })
      // invalidateQueries(['posts']) はprefix一致でこの投稿の詳細クエリ
      // (['posts','detail',postId])も対象にしてしまい、削除直後にまだ
      // マウントされているこのコンポーネントがそれを再フェッチして404を
      // 引き、navigate() 前に不要なエラー状態を作ってしまう。詳細クエリは
      // invalidateではなくキャッシュから除去し、一覧のみ再検証する。
      queryClient.removeQueries({ queryKey: ['posts', 'detail', postId] })
      await queryClient.invalidateQueries({ queryKey: ['posts', 'list'] })
      void navigate({ to: '/posts' })
    } catch (err) {
      if (err instanceof ApiError && err.status === 412) {
        setShowConflictBanner(true)
        return
      }
      if (err instanceof ApiError) {
        setGeneralError(toUserMessage(err.message))
      } else {
        setGeneralError('通信エラーが発生しました。')
      }
    }
  }

  if (postQuery.isLoading) {
    return <p>読み込み中...</p>
  }

  if (postQuery.isError) {
    if (postQuery.error instanceof ApiError && postQuery.error.status === 404) {
      return <p>記事が見つかりません。</p>
    }
    return <p role="alert">記事の取得に失敗しました。</p>
  }

  return (
    <div>
      <h1>投稿編集</h1>

      {generalError !== null && <p role="alert">{generalError}</p>}

      {showConflictBanner && (
        <div role="alert">
          <p>この投稿は他の場所で更新されています。最新の内容を読み込みますか?</p>
          <button type="button" onClick={() => void handleReloadLatest()}>
            最新を読み込む
          </button>
        </div>
      )}

      <form onSubmit={(event) => void handleSubmit(event)}>
        <div>
          <label htmlFor="post-title">タイトル</label>
          <input
            id="post-title"
            type="text"
            value={title}
            onChange={(event) => setTitle(event.target.value)}
          />
          {fieldErrors.title !== undefined && <p role="alert">{fieldErrors.title}</p>}
        </div>

        <div>
          <label htmlFor="post-slug">スラッグ</label>
          <input id="post-slug" type="text" value={slug} onChange={(event) => setSlug(event.target.value)} />
          {fieldErrors.slug !== undefined && <p role="alert">{fieldErrors.slug}</p>}
        </div>

        <div>
          <label htmlFor="post-body">本文</label>
          <textarea id="post-body" value={body} onChange={(event) => setBody(event.target.value)} />
          {fieldErrors.body !== undefined && <p role="alert">{fieldErrors.body}</p>}
        </div>

        <div>
          <label htmlFor="post-status">状態</label>
          <select
            id="post-status"
            value={status}
            onChange={(event) => setStatus(event.target.value as PostStatus)}
          >
            <option value="draft">下書き</option>
            <option value="published">公開済み</option>
          </select>
        </div>

        <button type="submit" disabled={updateMutation.isPending}>
          保存
        </button>
      </form>

      <button type="button" onClick={() => void handleDelete()} disabled={deleteMutation.isPending}>
        削除
      </button>
    </div>
  )
}
