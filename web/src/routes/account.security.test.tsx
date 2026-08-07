// 参照: docs/plans/p1-multi-user-rbac/design.md §5・§6.1・tasks.md T17
// 登録開始→確認→リカバリコード表示、無効化のコンポーネントテスト。
// fetchモックで完結させる(settings.test.tsxと同じ方針)。

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import AccountSecurity from './account.security'
import { ApiError, apiFetch } from '../api/client'

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
      <AccountSecurity />
    </QueryClientProvider>,
  )
}

describe('AccountSecurity', () => {
  beforeEach(() => {
    mockedApiFetch.mockReset()
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('starts enrollment and shows the secret and otpauth URI', async () => {
    mockedApiFetch.mockResolvedValueOnce({
      secret: 'JBSWY3DPEHPK3PXP',
      otpauth_uri: 'otpauth://totp/XordPress:admin?secret=JBSWY3DPEHPK3PXP',
    })
    const user = userEvent.setup()
    renderWithQueryClient()

    await user.type(screen.getByLabelText('現在のパスワード', { selector: '#security-start-password' }), 'password123')
    await user.click(screen.getByRole('button', { name: '登録を開始' }))

    expect(await screen.findByText('JBSWY3DPEHPK3PXP')).toBeInTheDocument()
    expect(mockedApiFetch).toHaveBeenCalledWith('/api/account/mfa', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ password: 'password123' }),
    })
  })

  it('shows an inline 409 error asking to disable first when already enrolled', async () => {
    mockedApiFetch.mockRejectedValueOnce(
      new ApiError(409, '2FA is already enabled; disable it before re-enrolling'),
    )
    const user = userEvent.setup()
    renderWithQueryClient()

    await user.type(screen.getByLabelText('現在のパスワード', { selector: '#security-start-password' }), 'password123')
    await user.click(screen.getByRole('button', { name: '登録を開始' }))

    expect(
      await screen.findByText('2段階認証は既に有効です。再登録する前に無効化してください。'),
    ).toBeInTheDocument()
  })

  it('confirms enrollment and shows recovery codes once', async () => {
    mockedApiFetch.mockResolvedValueOnce({
      secret: 'JBSWY3DPEHPK3PXP',
      otpauth_uri: 'otpauth://totp/XordPress:admin?secret=JBSWY3DPEHPK3PXP',
    })
    const user = userEvent.setup()
    renderWithQueryClient()

    await user.type(screen.getByLabelText('現在のパスワード', { selector: '#security-start-password' }), 'password123')
    await user.click(screen.getByRole('button', { name: '登録を開始' }))
    await screen.findByText('JBSWY3DPEHPK3PXP')

    mockedApiFetch.mockResolvedValueOnce({
      recovery_codes: ['aaaaa-11111', 'bbbbb-22222'],
    })
    await user.type(screen.getByLabelText('認証アプリに表示されたコード'), '123456')
    await user.click(screen.getByRole('button', { name: '確認' }))

    expect(await screen.findByText('aaaaa-11111')).toBeInTheDocument()
    expect(screen.getByText('bbbbb-22222')).toBeInTheDocument()
    expect(mockedApiFetch).toHaveBeenLastCalledWith('/api/account/mfa/confirm', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ code: '123456' }),
    })
  })

  it('shows an inline error when the confirmation code is wrong', async () => {
    mockedApiFetch.mockResolvedValueOnce({
      secret: 'JBSWY3DPEHPK3PXP',
      otpauth_uri: 'otpauth://totp/XordPress:admin?secret=JBSWY3DPEHPK3PXP',
    })
    const user = userEvent.setup()
    renderWithQueryClient()

    await user.type(screen.getByLabelText('現在のパスワード', { selector: '#security-start-password' }), 'password123')
    await user.click(screen.getByRole('button', { name: '登録を開始' }))
    await screen.findByText('JBSWY3DPEHPK3PXP')

    mockedApiFetch.mockRejectedValueOnce(new ApiError(401, 'invalid code'))
    await user.type(screen.getByLabelText('認証アプリに表示されたコード'), '000000')
    await user.click(screen.getByRole('button', { name: '確認' }))

    expect(await screen.findByText('コードが正しくありません。')).toBeInTheDocument()
  })

  it('disables 2FA and shows a success toast', async () => {
    mockedApiFetch.mockResolvedValueOnce(undefined)
    const user = userEvent.setup()
    renderWithQueryClient()

    await user.type(screen.getByLabelText('現在のパスワード', { selector: '#security-disable-password' }), 'password123')
    await user.click(screen.getByRole('button', { name: '無効化' }))

    expect(await screen.findByText('2段階認証を無効化しました。')).toBeInTheDocument()
    expect(mockedApiFetch).toHaveBeenCalledWith('/api/account/mfa', {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ password: 'password123' }),
    })
  })

  it('shows an inline error when disable fails due to a wrong password', async () => {
    mockedApiFetch.mockRejectedValueOnce(new ApiError(401, 'incorrect password'))
    const user = userEvent.setup()
    renderWithQueryClient()

    await user.type(screen.getByLabelText('現在のパスワード', { selector: '#security-disable-password' }), 'wrong')
    await user.click(screen.getByRole('button', { name: '無効化' }))

    expect(await screen.findByText('パスワードが正しくありません。')).toBeInTheDocument()
  })
})
