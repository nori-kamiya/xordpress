// 参照: docs/plans/admin-spa/design.md §5.3・§7
// slug自動生成、201遷移、409(slug衝突)、400系フィールドマッピングのコンポーネントテスト。

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import PostsNew from './posts.new'
import { apiFetch } from '../api/client'

const navigateMock = vi.fn()

vi.mock('@tanstack/react-router', async () => {
  const actual = await vi.importActual<typeof import('@tanstack/react-router')>('@tanstack/react-router')
  return {
    ...actual,
    useNavigate: () => navigateMock,
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
      <PostsNew />
    </QueryClientProvider>,
  )
}

async function fillForm(
  user: ReturnType<typeof userEvent.setup>,
  overrides: { title?: string; slug?: string; body?: string } = {},
): Promise<void> {
  if (overrides.title !== undefined) {
    await user.type(screen.getByLabelText('タイトル'), overrides.title)
  }
  if (overrides.slug !== undefined) {
    await user.clear(screen.getByLabelText('スラッグ'))
    await user.type(screen.getByLabelText('スラッグ'), overrides.slug)
  }
  await user.type(screen.getByLabelText('本文'), overrides.body ?? 'some body text')
}

describe('PostsNew', () => {
  beforeEach(() => {
    mockedApiFetch.mockReset()
    navigateMock.mockClear()
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('auto-generates the slug from the title', async () => {
    const user = userEvent.setup()
    renderWithQueryClient()

    await user.type(screen.getByLabelText('タイトル'), 'Hello World!')

    expect(screen.getByLabelText('スラッグ')).toHaveValue('hello-world')
  })

  it('stops auto-generating the slug once the user edits it manually', async () => {
    const user = userEvent.setup()
    renderWithQueryClient()

    await user.type(screen.getByLabelText('タイトル'), 'Hello World')
    await user.clear(screen.getByLabelText('スラッグ'))
    await user.type(screen.getByLabelText('スラッグ'), 'custom-slug')
    await user.type(screen.getByLabelText('タイトル'), ' Extra')

    expect(screen.getByLabelText('スラッグ')).toHaveValue('custom-slug')
  })

  it('creates the post and navigates to the edit page on 201', async () => {
    mockedApiFetch.mockResolvedValue(makePostResponse({ id: 42 }))
    const user = userEvent.setup()
    renderWithQueryClient()

    await fillForm(user, { title: 'Hello World', body: 'some body' })
    await user.click(screen.getByRole('button', { name: '作成' }))

    await waitFor(() =>
      expect(navigateMock).toHaveBeenCalledWith({
        to: '/posts/$postId/edit',
        params: { postId: '42' },
      }),
    )
  })

  it('shows an inline slug error on 409 (slug conflict)', async () => {
    const { ApiError } = await import('../api/client')
    mockedApiFetch.mockRejectedValue(new ApiError(409, 'slug already exists'))
    const user = userEvent.setup()
    renderWithQueryClient()

    await fillForm(user, { title: 'Hello World', body: 'some body' })
    await user.click(screen.getByRole('button', { name: '作成' }))

    expect(await screen.findByText('このスラッグは既に使用されています。')).toBeInTheDocument()
    expect(navigateMock).not.toHaveBeenCalled()
  })

  it('maps a 400 "invalid title" response to the title field', async () => {
    const { ApiError } = await import('../api/client')
    mockedApiFetch.mockRejectedValue(new ApiError(400, 'invalid title'))
    const user = userEvent.setup()
    renderWithQueryClient()

    await fillForm(user, { title: 'Hello World', body: 'some body' })
    await user.click(screen.getByRole('button', { name: '作成' }))

    expect(await screen.findByText('タイトルは1〜200文字で入力してください。')).toBeInTheDocument()
  })

  it('falls back to a general banner for an unmapped 400 response', async () => {
    const { ApiError } = await import('../api/client')
    mockedApiFetch.mockRejectedValue(new ApiError(400, 'invalid request body'))
    const user = userEvent.setup()
    renderWithQueryClient()

    await fillForm(user, { title: 'Hello World', body: 'some body' })
    await user.click(screen.getByRole('button', { name: '作成' }))

    expect(await screen.findByText('入力内容を確認してください。')).toBeInTheDocument()
  })
})
