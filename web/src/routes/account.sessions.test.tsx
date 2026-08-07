// 参照: docs/plans/p1-multi-user-rbac/design.md §5・tasks.md T16
// 一覧表示・現在セッションのマーキング・失効操作のコンポーネントテスト。
// fetchモックで完結させる(settings.test.tsxと同じ方針)。

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import AccountSessions from './account.sessions'
import { ApiError, apiFetch } from '../api/client'
import type { Session } from '../api/sessions'

vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return {
    ...actual,
    apiFetch: vi.fn(),
  }
})

const mockedApiFetch = vi.mocked(apiFetch)

function renderWithQueryClient(): void {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <AccountSessions />
    </QueryClientProvider>,
  )
}

const sessions: Session[] = [
  {
    id: 'aaaaaaaaaaaaaaaa',
    created_at: 1_700_000_000,
    expires_at: 1_700_604_800,
    user_agent: 'Mozilla/5.0',
    created_ip: '127.0.0.1',
    current: true,
  },
  {
    id: 'bbbbbbbbbbbbbbbb',
    created_at: 1_700_000_100,
    expires_at: 1_700_604_900,
    user_agent: 'curl/8.0',
    created_ip: '203.0.113.5',
    current: false,
  },
]

describe('AccountSessions', () => {
  beforeEach(() => {
    mockedApiFetch.mockReset()
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('lists sessions and marks the current one', async () => {
    mockedApiFetch.mockResolvedValueOnce(sessions)
    renderWithQueryClient()

    expect(await screen.findByText('Mozilla/5.0')).toBeInTheDocument()
    expect(screen.getByText('curl/8.0')).toBeInTheDocument()
    expect(screen.getByText('このセッション')).toBeInTheDocument()
  })

  it('shows a revoke button for every session, including the current one', async () => {
    mockedApiFetch.mockResolvedValueOnce(sessions)
    renderWithQueryClient()

    await screen.findByText('Mozilla/5.0')
    expect(screen.getAllByRole('button', { name: '失効' })).toHaveLength(2)
  })

  it('revokes the current session and lets the global 401 handler take over on refetch', async () => {
    mockedApiFetch.mockResolvedValueOnce(sessions)
    mockedApiFetch.mockResolvedValueOnce(undefined)
    mockedApiFetch.mockRejectedValueOnce(new ApiError(401, 'unauthorized'))
    const user = userEvent.setup()
    renderWithQueryClient()

    await screen.findByText('Mozilla/5.0')
    await user.click(screen.getAllByRole('button', { name: '失効' })[0]!)

    expect(mockedApiFetch).toHaveBeenNthCalledWith(2, '/api/account/sessions/aaaaaaaaaaaaaaaa', {
      method: 'DELETE',
    })
    // The refetch after invalidateQueries rejects with 401; apiFetch's own
    // global handler (not this component) is responsible for clearing the
    // cache and redirecting, so there is nothing further to assert here
    // beyond the DELETE call having been made with the current session's id.
  })

  it('revokes a non-current session and refetches the list', async () => {
    mockedApiFetch.mockResolvedValueOnce(sessions)
    mockedApiFetch.mockResolvedValueOnce(undefined)
    mockedApiFetch.mockResolvedValueOnce([sessions[0]])
    const user = userEvent.setup()
    renderWithQueryClient()

    await screen.findByText('curl/8.0')
    await user.click(screen.getAllByRole('button', { name: '失効' })[1]!)

    expect(mockedApiFetch).toHaveBeenNthCalledWith(2, '/api/account/sessions/bbbbbbbbbbbbbbbb', {
      method: 'DELETE',
    })
    expect(await screen.findByText('Mozilla/5.0')).toBeInTheDocument()
    expect(screen.queryByText('curl/8.0')).not.toBeInTheDocument()
  })

  it('shows a toast when revocation fails', async () => {
    mockedApiFetch.mockResolvedValueOnce(sessions)
    mockedApiFetch.mockRejectedValueOnce(new Error('network error'))
    const user = userEvent.setup()
    renderWithQueryClient()

    await screen.findByText('curl/8.0')
    await user.click(screen.getAllByRole('button', { name: '失効' })[1]!)

    expect(await screen.findByText('失効に失敗しました。')).toBeInTheDocument()
  })

  it('shows an error banner when the list fails to load', async () => {
    mockedApiFetch.mockRejectedValueOnce(new Error('network error'))
    renderWithQueryClient()

    expect(await screen.findByText('セッション一覧の取得に失敗しました。')).toBeInTheDocument()
  })
})
