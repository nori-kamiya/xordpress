// 参照: docs/plans/admin-spa/design.md §5.3・§7
//
// title/slug/body/status(draft/published)フォーム。POST /api/posts。
// - slug は title から自動生成するが、ユーザーが直接編集した後は自動生成を止める。
// - 201 -> /admin/posts/$id/edit へ遷移。
// - 409 (slug already exists) -> slug欄にインラインエラー。
// - 400系 -> errorMessages.ts でフィールド単位のエラーとして表示、対応表にない文字列は汎用バナーへ。

import { useState, type FormEvent } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useMutation } from '@tanstack/react-query'
import { createPostMutationOptions } from '../api/posts'
import { ApiError } from '../api/client'
import { toUserMessage } from '../api/errorMessages'
import { isValidTitle, isValidSlug, isValidBody } from '../api/validation'
import type { PostStatus } from '../types'

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

/**
 * title から slug を自動生成する。SLUG_PATTERN (英数字とハイフンのみ、
 * 先頭末尾ハイフンなし・連続ハイフンなし) を満たす形に正規化する。
 * (予約語チェックは isValidSlug 側の責務であり、ここでは行わない。)
 */
function generateSlug(title: string): string {
  return title
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

export default function PostsNew() {
  const navigate = useNavigate()
  const createMutation = useMutation(createPostMutationOptions())

  const [title, setTitle] = useState('')
  const [slug, setSlug] = useState('')
  const [slugManuallyEdited, setSlugManuallyEdited] = useState(false)
  const [body, setBody] = useState('')
  const [status, setStatus] = useState<PostStatus>('draft')
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({})
  const [generalError, setGeneralError] = useState<string | null>(null)

  function handleTitleChange(value: string): void {
    setTitle(value)
    if (!slugManuallyEdited) {
      setSlug(generateSlug(value))
    }
  }

  function handleSlugChange(value: string): void {
    setSlugManuallyEdited(true)
    setSlug(value)
  }

  function validate(): FieldErrors {
    const errors: FieldErrors = {}
    if (!isValidTitle(title)) {
      errors.title = toUserMessage('invalid title')
    }
    if (!isValidSlug(slug)) {
      errors.slug = toUserMessage('invalid slug')
    }
    if (!isValidBody(body)) {
      errors.body = toUserMessage('invalid body')
    }
    return errors
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault()
    setGeneralError(null)

    const clientErrors = validate()
    if (Object.keys(clientErrors).length > 0) {
      setFieldErrors(clientErrors)
      return
    }
    setFieldErrors({})

    try {
      const post = await createMutation.mutateAsync({ title, slug, body, status })
      void navigate({ to: '/posts/$postId/edit', params: { postId: String(post.id) } })
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.status === 409) {
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

  return (
    <div>
      <h1>投稿作成</h1>

      {generalError !== null && <p role="alert">{generalError}</p>}

      <form onSubmit={(event) => void handleSubmit(event)}>
        <div>
          <label htmlFor="post-title">タイトル</label>
          <input
            id="post-title"
            type="text"
            value={title}
            onChange={(event) => handleTitleChange(event.target.value)}
          />
          {fieldErrors.title !== undefined && <p role="alert">{fieldErrors.title}</p>}
        </div>

        <div>
          <label htmlFor="post-slug">スラッグ</label>
          <input
            id="post-slug"
            type="text"
            value={slug}
            onChange={(event) => handleSlugChange(event.target.value)}
          />
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

        <button type="submit" disabled={createMutation.isPending}>
          作成
        </button>
      </form>
    </div>
  )
}
