// 参照: docs/plans/p1-multi-user-rbac/design.md §5・tasks.md T15
// 一覧表示・追加・ロール変更・削除(reassign_to選択)のコンポーネントテスト。
// fetchモックで完結させる(account.sessions.test.tsxと同じ方針)。
// useCurrentUser(GET /api/account)も同じapiFetchモック経由で解決する。

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import UsersIndex from './users.index'
import { ApiError, apiFetch } from '../api/client'
import type { AdminUser } from '../api/users'
import type { CurrentUser } from '../api/account'

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
      <UsersIndex />
    </QueryClientProvider>,
  )
}

const currentUser: CurrentUser = {
  id: 1,
  username: 'admin',
  role: 'administrator',
  capabilities: ['users:manage'],
}

const users: AdminUser[] = [
  { id: 1, username: 'admin', role: 'administrator', created_at: 1_700_000_000 },
  { id: 2, username: 'editor-1', role: 'editor', created_at: 1_700_000_100 },
]

/** apiFetchはURLで分岐する。GET /api/account と GET /api/users の両方が
 * 同じuseQuery初回マウントで並行して呼ばれるため、mockImplementationで
 * パスに応じて振り分ける。 */
function mockAccountAndUsers(userList: AdminUser[] = users): void {
  mockedApiFetch.mockImplementation((path: string) => {
    if (path === '/api/account') {
      return Promise.resolve(currentUser)
    }
    if (path === '/api/users') {
      return Promise.resolve(userList)
    }
    return Promise.reject(new Error(`unexpected path: ${path}`))
  })
}

describe('UsersIndex', () => {
  beforeEach(() => {
    mockedApiFetch.mockReset()
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('lists users', async () => {
    mockAccountAndUsers()
    renderWithQueryClient()

    expect(await screen.findByRole('cell', { name: 'admin' })).toBeInTheDocument()
    expect(screen.getByText('editor-1')).toBeInTheDocument()
  })

  it('disables role change and delete for the current user own row', async () => {
    mockAccountAndUsers()
    renderWithQueryClient()

    await screen.findByRole('cell', { name: 'admin' })
    expect(screen.getByLabelText('adminのロール')).toBeDisabled()
    expect(screen.getAllByText('自分自身')).toHaveLength(1)
  })

  it('creates a new user and refetches the list', async () => {
    mockAccountAndUsers()
    const user = userEvent.setup()
    renderWithQueryClient()
    await screen.findByRole('cell', { name: 'admin' })

    mockedApiFetch.mockImplementationOnce((path: string, init?: { method?: string }) => {
      expect(path).toBe('/api/users')
      expect(init?.method).toBe('POST')
      return Promise.resolve({ id: 3, username: 'new-user', role: 'contributor', created_at: 1_700_000_200 })
    })
    mockAccountAndUsers([...users, { id: 3, username: 'new-user', role: 'contributor', created_at: 1_700_000_200 }])

    await user.type(screen.getByLabelText('ユーザー名'), 'new-user')
    await user.type(screen.getByLabelText('パスワード'), 'password123')
    await user.click(screen.getByRole('button', { name: '追加' }))

    expect(await screen.findByRole('cell', { name: 'new-user' })).toBeInTheDocument()
  })

  it('shows a validation error for an invalid username without calling the API', async () => {
    mockAccountAndUsers()
    const user = userEvent.setup()
    renderWithQueryClient()
    await screen.findByRole('cell', { name: 'admin' })

    const callCountBefore = mockedApiFetch.mock.calls.length

    await user.type(screen.getByLabelText('ユーザー名'), 'a')
    await user.type(screen.getByLabelText('パスワード'), 'password123')
    await user.click(screen.getByRole('button', { name: '追加' }))

    expect(
      await screen.findByText('ユーザー名は3〜32文字の英数字・アンダースコア・ハイフンで入力してください。'),
    ).toBeInTheDocument()
    expect(mockedApiFetch.mock.calls.length).toBe(callCountBefore)
  })

  it('shows the server error inline when creation fails with a duplicate username', async () => {
    mockAccountAndUsers()
    const user = userEvent.setup()
    renderWithQueryClient()
    await screen.findByRole('cell', { name: 'admin' })

    mockedApiFetch.mockRejectedValueOnce(new ApiError(409, 'username already taken'))

    await user.type(screen.getByLabelText('ユーザー名'), 'admin')
    await user.type(screen.getByLabelText('パスワード'), 'password123')
    await user.click(screen.getByRole('button', { name: '追加' }))

    expect(await screen.findByText('このユーザー名は既に使用されています。')).toBeInTheDocument()
  })

  it('changes another user role', async () => {
    mockAccountAndUsers()
    const user = userEvent.setup()
    renderWithQueryClient()
    await screen.findByText('editor-1')

    mockedApiFetch.mockImplementationOnce((path: string, init?: { method?: string }) => {
      expect(path).toBe('/api/users/2')
      expect(init?.method).toBe('PATCH')
      return Promise.resolve({ id: 2, username: 'editor-1', role: 'author', created_at: 1_700_000_100 })
    })
    mockAccountAndUsers([users[0]!, { id: 2, username: 'editor-1', role: 'author', created_at: 1_700_000_100 }])

    await user.selectOptions(screen.getByLabelText('editor-1のロール'), '投稿者')

    await screen.findByText('editor-1')
  })

  it('requires a reassign_to selection before deleting', async () => {
    mockAccountAndUsers()
    const user = userEvent.setup()
    renderWithQueryClient()
    await screen.findByText('editor-1')

    const callCountBefore = mockedApiFetch.mock.calls.length
    await user.click(screen.getByRole('button', { name: '削除' }))

    expect(await screen.findByText('引き継ぎ先のユーザーを選択してください。')).toBeInTheDocument()
    expect(mockedApiFetch.mock.calls.length).toBe(callCountBefore)
  })

  it('re-selecting the placeholder after choosing a target requires a selection again', async () => {
    mockAccountAndUsers()
    const user = userEvent.setup()
    renderWithQueryClient()
    await screen.findByText('editor-1')

    await user.selectOptions(screen.getByLabelText('記事の引き継ぎ先'), 'admin')
    await user.selectOptions(screen.getByLabelText('記事の引き継ぎ先'), '選択してください')

    const callCountBefore = mockedApiFetch.mock.calls.length
    await user.click(screen.getByRole('button', { name: '削除' }))

    expect(await screen.findByText('引き継ぎ先のユーザーを選択してください。')).toBeInTheDocument()
    expect(mockedApiFetch.mock.calls.length).toBe(callCountBefore)
  })

  it('deletes a user after selecting a reassign target and refetches the list', async () => {
    mockAccountAndUsers()
    const user = userEvent.setup()
    renderWithQueryClient()
    await screen.findByText('editor-1')

    mockedApiFetch.mockImplementationOnce((path: string, init?: { method?: string }) => {
      expect(path).toBe('/api/users/2?reassign_to=1')
      expect(init?.method).toBe('DELETE')
      return Promise.resolve(undefined)
    })
    mockAccountAndUsers([users[0]!])

    await user.selectOptions(screen.getByLabelText('記事の引き継ぎ先'), 'admin')
    await user.click(screen.getByRole('button', { name: '削除' }))

    expect(await screen.findByRole('cell', { name: 'admin' })).toBeInTheDocument()
    expect(screen.queryByText('editor-1')).not.toBeInTheDocument()
  })

  it('shows an error banner when the list fails to load', async () => {
    mockedApiFetch.mockImplementation((path: string) => {
      if (path === '/api/account') {
        return Promise.resolve(currentUser)
      }
      return Promise.reject(new Error('network error'))
    })
    renderWithQueryClient()

    expect(await screen.findByText('ユーザー一覧の取得に失敗しました。')).toBeInTheDocument()
  })
})
