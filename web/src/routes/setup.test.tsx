// 参照: docs/plans/admin-spa/design.md §5.1・§7
// フォーム検証・201/409/400分岐のコンポーネントテスト。

import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import Setup from './setup'
import { ApiError } from '../api/client'
import { toUserMessage } from '../api/errorMessages'
import { setup } from '../api/setup'

const navigateMock = vi.fn()

vi.mock('@tanstack/react-router', async () => {
  const actual = await vi.importActual<typeof import('@tanstack/react-router')>('@tanstack/react-router')
  return {
    ...actual,
    useNavigate: () => navigateMock,
    Link: ({ to, children }: { to: string; children: ReactNode }) => <a href={to}>{children}</a>,
  }
})

vi.mock('../api/setup', () => ({
  setup: vi.fn(),
}))

async function fillValidForm(
  user: ReturnType<typeof userEvent.setup>,
  overrides: { token?: string; username?: string; password?: string; siteName?: string } = {},
): Promise<void> {
  await user.type(screen.getByLabelText('セットアップトークン'), overrides.token ?? 'tok-123')
  await user.type(screen.getByLabelText('ユーザー名'), overrides.username ?? 'alice')
  await user.type(screen.getByLabelText('パスワード'), overrides.password ?? 'password123')
  await user.type(screen.getByLabelText('サイト名'), overrides.siteName ?? 'My Site')
}

describe('Setup', () => {
  beforeEach(() => {
    navigateMock.mockClear()
    vi.mocked(setup).mockReset()
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('rejects an empty submission with client-side field errors and does not call the API', async () => {
    const user = userEvent.setup()
    render(<Setup />)

    await user.click(screen.getByRole('button', { name: 'セットアップを完了する' }))

    expect(await screen.findByText('セットアップトークンを入力してください。')).toBeInTheDocument()
    expect(screen.getByText(toUserMessage('invalid username'))).toBeInTheDocument()
    expect(screen.getByText(toUserMessage('invalid password'))).toBeInTheDocument()
    expect(setup).not.toHaveBeenCalled()
  })

  it('submits and navigates to /login?setup=done on 201', async () => {
    vi.mocked(setup).mockResolvedValue(undefined)
    const user = userEvent.setup()
    render(<Setup />)

    await fillValidForm(user)
    await user.click(screen.getByRole('button', { name: 'セットアップを完了する' }))

    await waitFor(() =>
      expect(navigateMock).toHaveBeenCalledWith({ to: '/login', search: { setup: 'done' } }),
    )
    expect(setup).toHaveBeenCalledWith('tok-123', {
      username: 'alice',
      password: 'password123',
      site_name: 'My Site',
    })
  })

  it('clears the token field after submit even when the request fails', async () => {
    vi.mocked(setup).mockRejectedValue(new ApiError(500, 'internal error'))
    const user = userEvent.setup()
    render(<Setup />)

    await fillValidForm(user)
    await user.click(screen.getByRole('button', { name: 'セットアップを完了する' }))

    await waitFor(() => expect(screen.getByLabelText('セットアップトークン')).toHaveValue(''))
  })

  it('shows an inline "already set up" message with a login link on 409', async () => {
    vi.mocked(setup).mockRejectedValue(new ApiError(409, 'setup already completed'))
    const user = userEvent.setup()
    render(<Setup />)

    await fillValidForm(user)
    await user.click(screen.getByRole('button', { name: 'セットアップを完了する' }))

    expect(await screen.findByText(/セットアップ済みです。ログインしてください。/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'ログインへ' })).toHaveAttribute('href', '/login')
    expect(navigateMock).not.toHaveBeenCalled()
  })

  it.each([
    ['unauthorized' as const, 'セットアップトークン'],
    ['invalid username' as const, 'ユーザー名'],
    ['invalid password' as const, 'パスワード'],
    ['invalid site_name' as const, 'サイト名'],
  ])('maps server message %s onto the %s field', async (serverMessage, fieldLabel) => {
    vi.mocked(setup).mockRejectedValue(new ApiError(400, serverMessage))
    const user = userEvent.setup()
    render(<Setup />)

    await fillValidForm(user)
    await user.click(screen.getByRole('button', { name: 'セットアップを完了する' }))

    expect(await screen.findByText(toUserMessage(serverMessage))).toBeInTheDocument()
    // 対象フィールドのラベルはフォーム上に存在し続ける(フィールド単位表示の確認)。
    expect(screen.getByLabelText(fieldLabel)).toBeInTheDocument()
  })

  it('shows a generic banner for a 400 with no field mapping (invalid request body)', async () => {
    vi.mocked(setup).mockRejectedValue(new ApiError(400, 'invalid request body'))
    const user = userEvent.setup()
    render(<Setup />)

    await fillValidForm(user)
    await user.click(screen.getByRole('button', { name: 'セットアップを完了する' }))

    expect(await screen.findByText(toUserMessage('invalid request body'))).toBeInTheDocument()
  })
})
