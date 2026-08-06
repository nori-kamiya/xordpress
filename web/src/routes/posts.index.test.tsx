// 参照: docs/plans/admin-spa/design.md §5.3
// 一覧表示・statusフィルタ・削除の200/412分岐のコンポーネントテスト。

import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import PostsIndex from './posts.index'
import { ApiError, apiFetch } from '../api/client'
import type { PostListResponse } from '../types'

vi.mock('@tanstack/react-router', async () => {
  const actual = await vi.importActual<typeof import('@tanstack/react-router')>('@tanstack/react-router')
  return {
    ...actual,
    Link: ({ to, params, children }: { to: string; params?: Record<string, string>; children: ReactNode }) => {
      const href = params
        ? Object.entries(params).reduce((acc, [key, value]) => acc.replace(`$${key}`, value), to)
        : to
      return <a href={href}>{children}</a>
    },
  }
})

vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return {
    ...actual,
    apiFetch: vi.fn(),
  }
})

const mockedApiFetch = vi.mocked(apiFetch)

function makeListResponse(overrides: Partial<PostListResponse> = {}): PostListResponse {
  return {
    items: [
      {
        id: 1,
        slug: 'hello-world',
        title: 'Hello World',
        status: 'draft',
        version: 1,
        updated_at: 1700000000,
      },
      {
        id: 2,
        slug: 'second-post',
        title: 'Second Post',
        status: 'published',
        version: 3,
        updated_at: 1700000100,
      },
    ],
    total: 2,
    ...overrides,
  }
}

function renderWithQueryClient(): QueryClient {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <PostsIndex />
    </QueryClientProvider>,
  )
  return queryClient
}

describe('PostsIndex', () => {
  beforeEach(() => {
    mockedApiFetch.mockReset()
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('renders the post list table with title/status/updated_at columns', async () => {
    mockedApiFetch.mockResolvedValue(makeListResponse())
    renderWithQueryClient()

    expect(await screen.findByText('Hello World')).toBeInTheDocument()
    expect(screen.getByText('Second Post')).toBeInTheDocument()
    const rows = screen.getAllByRole('row')
    expect(rows[1]).toHaveTextContent('下書き')
    expect(rows[2]).toHaveTextContent('公開済み')
  })

  it('links to /posts/new for the new-post action', async () => {
    mockedApiFetch.mockResolvedValue(makeListResponse())
    renderWithQueryClient()

    await screen.findByText('Hello World')
    expect(screen.getByRole('link', { name: '新規作成' })).toHaveAttribute('href', '/posts/new')
  })

  it('links each row to its edit page', async () => {
    mockedApiFetch.mockResolvedValue(makeListResponse())
    renderWithQueryClient()

    await screen.findByText('Hello World')
    const [firstEditLink, secondEditLink] = screen.getAllByRole('link', { name: '編集' })
    expect(firstEditLink).toHaveAttribute('href', '/posts/1/edit')
    expect(secondEditLink).toHaveAttribute('href', '/posts/2/edit')
  })

  it('refetches with the selected status filter', async () => {
    mockedApiFetch.mockResolvedValue(makeListResponse())
    const user = userEvent.setup()
    renderWithQueryClient()

    await screen.findByText('Hello World')
    mockedApiFetch.mockResolvedValue(makeListResponse({ items: [], total: 0 }))

    await user.selectOptions(screen.getByLabelText('状態'), '下書き')

    await waitFor(() =>
      expect(mockedApiFetch).toHaveBeenLastCalledWith('/api/posts?status=draft&limit=20&offset=0'),
    )
  })

  it('deletes a post and invalidates the posts query on success', async () => {
    mockedApiFetch.mockImplementation((_path: string, init?: { method?: string }) => {
      if (init?.method === 'DELETE') {
        return Promise.resolve(undefined)
      }
      return Promise.resolve(makeListResponse())
    })
    const user = userEvent.setup()
    const queryClient = renderWithQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    await screen.findByText('Hello World')
    const firstDeleteButton = screen.getAllByRole('button', { name: '削除' }).at(0)
    if (firstDeleteButton === undefined) {
      throw new Error('expected at least one delete button')
    }
    await user.click(firstDeleteButton)

    await waitFor(() =>
      expect(mockedApiFetch).toHaveBeenCalledWith(
        '/api/posts/1',
        expect.objectContaining({ method: 'DELETE', headers: { 'If-Match': '"1"' } }),
      ),
    )
    await waitFor(() => expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['posts'] }))
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  it('shows a conflict toast and refetches on 412 without retrying', async () => {
    let deleteCallCount = 0
    mockedApiFetch.mockImplementation((_path: string, init?: { method?: string }) => {
      if (init?.method === 'DELETE') {
        deleteCallCount += 1
        return Promise.reject(new ApiError(412, 'version mismatch'))
      }
      return Promise.resolve(makeListResponse())
    })
    const user = userEvent.setup()
    const queryClient = renderWithQueryClient()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    await screen.findByText('Hello World')
    const firstDeleteButton = screen.getAllByRole('button', { name: '削除' }).at(0)
    if (firstDeleteButton === undefined) {
      throw new Error('expected at least one delete button')
    }
    await user.click(firstDeleteButton)

    expect(await screen.findByRole('status')).toHaveTextContent('この投稿は変更されています')
    expect(deleteCallCount).toBe(1)
    await waitFor(() => expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['posts'] }))
  })
})
