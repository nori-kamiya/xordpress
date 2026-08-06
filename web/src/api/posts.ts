// 参照: docs/plans/admin-spa/design.md §4.2 (posts系エンドポイント)

import { mutationOptions, queryOptions } from '@tanstack/react-query'
import { apiFetch } from './client'
import type { Post, PostListResponse, PostStatus, PostWriteRequest } from '../types'

export interface ListPostsParams {
  status?: PostStatus
  limit?: number
  offset?: number
}

function buildListQuery(params: ListPostsParams): string {
  const search = new URLSearchParams()
  if (params.status !== undefined) {
    search.set('status', params.status)
  }
  if (params.limit !== undefined) {
    search.set('limit', String(params.limit))
  }
  if (params.offset !== undefined) {
    search.set('offset', String(params.offset))
  }
  const qs = search.toString()
  return qs.length > 0 ? `?${qs}` : ''
}

/** If-Match ヘッダーは "${version}" 形式(ダブルクォート付き)で組み立てる。 */
export function ifMatchHeader(version: number): string {
  return `"${version}"`
}

export function listPosts(params: ListPostsParams = {}): Promise<PostListResponse> {
  return apiFetch<PostListResponse>(`/api/posts${buildListQuery(params)}`)
}

export const postsListQueryOptions = (params: ListPostsParams = {}) =>
  queryOptions({
    queryKey: ['posts', 'list', params] as const,
    queryFn: () => listPosts(params),
  })

export function getPost(id: string): Promise<Post> {
  return apiFetch<Post>(`/api/posts/${id}`)
}

export const postQueryOptions = (id: string) =>
  queryOptions({
    queryKey: ['posts', 'detail', id] as const,
    queryFn: () => getPost(id),
  })

export function createPost(payload: PostWriteRequest): Promise<Post> {
  return apiFetch<Post>('/api/posts', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
}

export const createPostMutationOptions = () =>
  mutationOptions({
    mutationKey: ['posts', 'create'] as const,
    mutationFn: createPost,
  })

export interface UpdatePostVariables {
  id: string
  version: number
  payload: PostWriteRequest
}

export function updatePost({ id, version, payload }: UpdatePostVariables): Promise<Post> {
  return apiFetch<Post>(`/api/posts/${id}`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      'If-Match': ifMatchHeader(version),
    },
    body: JSON.stringify(payload),
  })
}

export const updatePostMutationOptions = () =>
  mutationOptions({
    mutationKey: ['posts', 'update'] as const,
    mutationFn: updatePost,
  })

export interface DeletePostVariables {
  id: string
  version: number
}

export function deletePost({ id, version }: DeletePostVariables): Promise<void> {
  return apiFetch<void>(`/api/posts/${id}`, {
    method: 'DELETE',
    headers: { 'If-Match': ifMatchHeader(version) },
  })
}

export const deletePostMutationOptions = () =>
  mutationOptions({
    mutationKey: ['posts', 'delete'] as const,
    mutationFn: deletePost,
  })
