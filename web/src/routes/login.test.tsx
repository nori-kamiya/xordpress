// 参照: docs/plans/admin-spa/design.md §5.1・§5.2
// 204/401分岐、redirect遷移、setup=doneバナーのコンポーネントテスト。

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import Login from './login'
import { ApiError } from '../api/client'
import { login } from '../api/session'

const navigateMock = vi.fn()
const invalidateQueriesMock = vi.fn()
let searchParams: { redirect?: string; setup?: string } = {}

vi.mock('@tanstack/react-router', async () => {
  const actual = await vi.importActual<typeof import('@tanstack/react-router')>('@tanstack/react-router')
  return {
    ...actual,
    useNavigate: () => navigateMock,
    useSearch: () => searchParams,
  }
})

vi.mock('@tanstack/react-query', async () => {
  const actual = await vi.importActual<typeof import('@tanstack/react-query')>('@tanstack/react-query')
  return {
    ...actual,
    useQueryClient: () => ({ invalidateQueries: invalidateQueriesMock }),
  }
})

vi.mock('../api/session', () => ({
  login: vi.fn(),
}))

async function fillForm(
  user: ReturnType<typeof userEvent.setup>,
  overrides: { username?: string; password?: string } = {},
): Promise<void> {
  await user.type(screen.getByLabelText('ユーザー名'), overrides.username ?? 'alice')
  await user.type(screen.getByLabelText('パスワード'), overrides.password ?? 'password123')
}

describe('Login', () => {
  beforeEach(() => {
    searchParams = {}
    navigateMock.mockClear()
    invalidateQueriesMock.mockClear()
    vi.mocked(login).mockReset()
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('invalidates posts queries and navigates to /posts on 204 with no redirect param', async () => {
    vi.mocked(login).mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(<Login />)

    await fillForm(user)
    await user.click(screen.getByRole('button', { name: 'ログイン' }))

    await waitFor(() => expect(navigateMock).toHaveBeenCalledWith({ to: '/posts' }))
    expect(invalidateQueriesMock).toHaveBeenCalledWith({ queryKey: ['posts'] })
    expect(login).toHaveBeenCalledWith({ username: 'alice', password: 'password123' })
  })

  it('navigates to the redirect target when ?redirect= is present', async () => {
    searchParams = { redirect: '/settings' }
    vi.mocked(login).mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(<Login />)

    await fillForm(user)
    await user.click(screen.getByRole('button', { name: 'ログイン' }))

    await waitFor(() => expect(navigateMock).toHaveBeenCalledWith({ href: '/settings' }))
  })

  it('shows an inline error and does not navigate on 401', async () => {
    vi.mocked(login).mockRejectedValue(new ApiError(401, 'unauthorized'))
    const user = userEvent.setup()
    render(<Login />)

    await fillForm(user)
    await user.click(screen.getByRole('button', { name: 'ログイン' }))

    expect(await screen.findByText('ユーザー名またはパスワードが違います。')).toBeInTheDocument()
    expect(navigateMock).not.toHaveBeenCalled()
    expect(invalidateQueriesMock).not.toHaveBeenCalled()
  })

  it('shows a generic error banner for non-401 failures', async () => {
    vi.mocked(login).mockRejectedValue(new ApiError(500, 'internal error'))
    const user = userEvent.setup()
    render(<Login />)

    await fillForm(user)
    await user.click(screen.getByRole('button', { name: 'ログイン' }))

    expect(
      await screen.findByText('サーバーエラーが発生しました。しばらくしてから再度お試しください。'),
    ).toBeInTheDocument()
  })

  it('shows a success banner when ?setup=done', () => {
    searchParams = { setup: 'done' }
    render(<Login />)

    expect(screen.getByText('セットアップが完了しました。ログインしてください。')).toBeInTheDocument()
  })
})
