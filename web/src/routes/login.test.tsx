// 参照: docs/plans/admin-spa/design.md §5.1・§5.2
// 204/401分岐、redirect遷移、setup=doneバナーのコンポーネントテスト。

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import Login from './login'
import { ApiError } from '../api/client'
import { login } from '../api/session'
import { verifyMFA } from '../api/mfa'

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

vi.mock('../api/mfa', () => ({
  verifyMFA: vi.fn(),
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
    vi.mocked(verifyMFA).mockReset()
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

  it.each(['https://evil.example/phish', '//evil.example/phish', '/\\evil.example/phish'])(
    'falls back to /posts instead of an open redirect for unsafe ?redirect=%s',
    async (unsafeTarget) => {
      searchParams = { redirect: unsafeTarget }
      vi.mocked(login).mockResolvedValue(undefined)
      const user = userEvent.setup()
      render(<Login />)

      await fillForm(user)
      await user.click(screen.getByRole('button', { name: 'ログイン' }))

      await waitFor(() => expect(navigateMock).toHaveBeenCalledWith({ to: '/posts' }))
      expect(navigateMock).not.toHaveBeenCalledWith({ href: unsafeTarget })
    },
  )

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

  it('shows the lockout message for a 429 (roadmap-to-wp-parity.md P0-3)', async () => {
    vi.mocked(login).mockRejectedValue(new ApiError(429, 'too many failed login attempts, try again later'))
    const user = userEvent.setup()
    render(<Login />)

    await fillForm(user)
    await user.click(screen.getByRole('button', { name: 'ログイン' }))

    expect(
      await screen.findByText('ログイン試行回数が多すぎます。しばらくしてから再度お試しください。'),
    ).toBeInTheDocument()
    expect(navigateMock).not.toHaveBeenCalled()
  })

  it('shows a success banner when ?setup=done', () => {
    searchParams = { setup: 'done' }
    render(<Login />)

    expect(screen.getByText('セットアップが完了しました。ログインしてください。')).toBeInTheDocument()
  })

  it('switches to the code form on mfa_required and completes login on a valid code', async () => {
    vi.mocked(login).mockResolvedValue({ mfa_required: true, mfa_token: 'raw-mfa-token' })
    vi.mocked(verifyMFA).mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(<Login />)

    await fillForm(user)
    await user.click(screen.getByRole('button', { name: 'ログイン' }))

    expect(await screen.findByLabelText('認証アプリのコード、またはリカバリコード')).toBeInTheDocument()
    expect(navigateMock).not.toHaveBeenCalled()

    await user.type(screen.getByLabelText('認証アプリのコード、またはリカバリコード'), '123456')
    await user.click(screen.getByRole('button', { name: '確認' }))

    expect(verifyMFA).toHaveBeenCalledWith('raw-mfa-token', '123456')
    await waitFor(() => expect(navigateMock).toHaveBeenCalledWith({ to: '/posts' }))
    expect(invalidateQueriesMock).toHaveBeenCalledWith({ queryKey: ['posts'] })
  })

  it('shows an inline error and stays on the code form when the code is wrong', async () => {
    vi.mocked(login).mockResolvedValue({ mfa_required: true, mfa_token: 'raw-mfa-token' })
    vi.mocked(verifyMFA).mockRejectedValue(new ApiError(401, 'invalid code'))
    const user = userEvent.setup()
    render(<Login />)

    await fillForm(user)
    await user.click(screen.getByRole('button', { name: 'ログイン' }))
    await screen.findByLabelText('認証アプリのコード、またはリカバリコード')

    await user.type(screen.getByLabelText('認証アプリのコード、またはリカバリコード'), '000000')
    await user.click(screen.getByRole('button', { name: '確認' }))

    expect(await screen.findByText('コードが正しくありません。')).toBeInTheDocument()
    expect(navigateMock).not.toHaveBeenCalled()
  })

  it('respects the redirect target after completing the 2FA step', async () => {
    searchParams = { redirect: '/settings' }
    vi.mocked(login).mockResolvedValue({ mfa_required: true, mfa_token: 'raw-mfa-token' })
    vi.mocked(verifyMFA).mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(<Login />)

    await fillForm(user)
    await user.click(screen.getByRole('button', { name: 'ログイン' }))
    await screen.findByLabelText('認証アプリのコード、またはリカバリコード')

    await user.type(screen.getByLabelText('認証アプリのコード、またはリカバリコード'), '123456')
    await user.click(screen.getByRole('button', { name: '確認' }))

    await waitFor(() => expect(navigateMock).toHaveBeenCalledWith({ href: '/settings' }))
  })
})
