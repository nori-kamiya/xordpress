// 参照: docs/plans/admin-spa/design.md §5.3・§7
// 404、412衝突バナー+「最新を読み込む」、409(slug衝突)、削除の分岐テスト。

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import PostsEdit from './posts.edit'
import { apiFetch } from '../api/client'

const navigateMock = vi.fn()

vi.mock('@tanstack/react-router', async () => {
  const actual = await vi.importActual<typeof import('@tanstack/react-router')>('@tanstack/react-router')
  return {
    ...actual,
    useNavigate: () => navigateMock,
    useParams: () => ({ postId: '42' }),
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

function makePostResponse(overrides: Partial<Record<string, unknown>> = {}): Record<string, unknown> {
  return {
    id: 42,
    slug: 'hello-world',
    title: 'Hello World',
    body: 'body text',
    status: 'draft',
    version: 1,
    author_id: 1,
    created_at: 1700000000,
    updated_at: 1700000000,
    published_at: null,
    ...overrides,
  }
}

function renderWithQueryClient(): void {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <PostsEdit />
    </QueryClientProvider>,
  )
}

describe('PostsEdit', () => {
  beforeEach(() => {
    mockedApiFetch.mockReset()
    navigateMock.mockClear()
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('shows a not-found message on 404', async () => {
    const { ApiError } = await import('../api/client')
    mockedApiFetch.mockRejectedValue(new ApiError(404, 'post not found'))
    renderWithQueryClient()

    expect(await screen.findByText('記事が見つかりません。')).toBeInTheDocument()
  })

  it('loads the post and populates the form', async () => {
    mockedApiFetch.mockResolvedValue(makePostResponse())
    renderWithQueryClient()

    expect(await screen.findByLabelText('タイトル')).toHaveValue('Hello World')
    expect(screen.getByLabelText('スラッグ')).toHaveValue('hello-world')
    expect(screen.getByLabelText('本文')).toHaveValue('body text')
  })

  it('shows the conflict banner on 412 and reloads the latest content', async () => {
    mockedApiFetch.mockResolvedValueOnce(makePostResponse({ version: 1 }))
    const { ApiError } = await import('../api/client')
    const user = userEvent.setup()
    renderWithQueryClient()

    await screen.findByLabelText('タイトル')

    mockedApiFetch.mockRejectedValueOnce(new ApiError(412, 'version mismatch'))
    await user.click(screen.getByRole('button', { name: '保存' }))

    expect(
      await screen.findByText('この投稿は他の場所で更新されています。最新の内容を読み込みますか?'),
    ).toBeInTheDocument()

    mockedApiFetch.mockResolvedValueOnce(
      makePostResponse({ version: 2, title: 'Updated Elsewhere' }),
    )
    await user.click(screen.getByRole('button', { name: '最新を読み込む' }))

    await waitFor(() => expect(screen.getByLabelText('タイトル')).toHaveValue('Updated Elsewhere'))
    expect(
      screen.queryByText('この投稿は他の場所で更新されています。最新の内容を読み込みますか?'),
    ).not.toBeInTheDocument()
  })

  it('shows an inline slug error on 409 (slug conflict)', async () => {
    mockedApiFetch.mockResolvedValueOnce(makePostResponse())
    const { ApiError } = await import('../api/client')
    const user = userEvent.setup()
    renderWithQueryClient()

    await screen.findByLabelText('タイトル')

    mockedApiFetch.mockRejectedValueOnce(new ApiError(409, 'slug already exists'))
    await user.click(screen.getByRole('button', { name: '保存' }))

    expect(await screen.findByText('このスラッグは既に使用されています。')).toBeInTheDocument()
  })

  it('deletes the post and navigates to /posts on success', async () => {
    mockedApiFetch.mockResolvedValueOnce(makePostResponse())
    mockedApiFetch.mockResolvedValueOnce(undefined)
    const user = userEvent.setup()
    renderWithQueryClient()

    await screen.findByLabelText('タイトル')

    await user.click(screen.getByRole('button', { name: '削除' }))

    await waitFor(() => expect(navigateMock).toHaveBeenCalledWith({ to: '/posts' }))
  })
})
