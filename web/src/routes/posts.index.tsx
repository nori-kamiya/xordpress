// 参照: docs/plans/admin-spa/design.md §5.3
//
// GET /api/posts の一覧テーブル。status フィルタ・limit/offset ページネーション。
// 削除は DELETE + If-Match。412 (version mismatch) を検知したら
// 「この投稿は変更されています」トーストを表示し、一覧を invalidateQueries(['posts'])
// で再フェッチして終了する(自動リトライはしない)。

import { useMemo, useState, type ChangeEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ApiError } from '../api/client'
import { deletePostMutationOptions, postsListQueryOptions } from '../api/posts'
import type { PostStatus, PostSummary } from '../types'

const PAGE_SIZE = 20

type StatusFilter = '' | PostStatus

export default function PostsIndex() {
  const queryClient = useQueryClient()
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('')
  const [offset, setOffset] = useState(0)
  const [toastMessage, setToastMessage] = useState<string | null>(null)

  const listParams = useMemo(
    () => ({
      status: statusFilter === '' ? undefined : statusFilter,
      limit: PAGE_SIZE,
      offset,
    }),
    [statusFilter, offset],
  )

  const { data, isLoading, isError } = useQuery(postsListQueryOptions(listParams))
  const deleteMutation = useMutation(deletePostMutationOptions())

  function handleStatusFilterChange(event: ChangeEvent<HTMLSelectElement>): void {
    setStatusFilter(event.target.value as StatusFilter)
    setOffset(0)
  }

  async function handleDelete(post: PostSummary): Promise<void> {
    setToastMessage(null)
    try {
      await deleteMutation.mutateAsync({ id: String(post.id), version: post.version })
      await queryClient.invalidateQueries({ queryKey: ['posts'] })
    } catch (err) {
      if (err instanceof ApiError && err.status === 412) {
        setToastMessage('この投稿は変更されています')
        await queryClient.invalidateQueries({ queryKey: ['posts'] })
        return
      }
      setToastMessage('削除に失敗しました。')
    }
  }

  const items = data?.items ?? []
  const total = data?.total ?? 0
  const hasPrevPage = offset > 0
  const hasNextPage = offset + PAGE_SIZE < total

  return (
    <div>
      <h1>投稿一覧</h1>

      {toastMessage !== null && <p role="status">{toastMessage}</p>}

      <div>
        <label htmlFor="posts-status-filter">状態</label>
        <select id="posts-status-filter" value={statusFilter} onChange={handleStatusFilterChange}>
          <option value="">すべて</option>
          <option value="draft">下書き</option>
          <option value="published">公開済み</option>
        </select>

        <Link to="/posts/new">新規作成</Link>
      </div>

      {isLoading && <p>読み込み中...</p>}
      {isError && <p role="alert">一覧の取得に失敗しました。</p>}

      {!isLoading && !isError && (
        <table>
          <thead>
            <tr>
              <th>タイトル</th>
              <th>状態</th>
              <th>更新日時</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {items.map((post) => (
              <tr key={post.id}>
                <td>{post.title}</td>
                <td>{post.status === 'draft' ? '下書き' : '公開済み'}</td>
                <td>{new Date(post.updated_at * 1000).toLocaleString()}</td>
                <td>
                  <Link to="/posts/$postId/edit" params={{ postId: String(post.id) }}>
                    編集
                  </Link>
                  <button
                    type="button"
                    onClick={() => void handleDelete(post)}
                    disabled={deleteMutation.isPending}
                  >
                    削除
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <div>
        <button
          type="button"
          onClick={() => setOffset((prev) => Math.max(0, prev - PAGE_SIZE))}
          disabled={!hasPrevPage}
        >
          前へ
        </button>
        <button type="button" onClick={() => setOffset((prev) => prev + PAGE_SIZE)} disabled={!hasNextPage}>
          次へ
        </button>
      </div>
    </div>
  )
}
