// 参照: docs/plans/admin-spa/design.md §4.1/§4.2・§5.3
// 表示(GET)・更新(PUT)・検証エラー分岐のコンポーネントテスト。fetchモックで完結させる
// (統合確認はT12で行う)。

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Settings from './settings'
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
      <Settings />
    </QueryClientProvider>,
  )
}

describe('Settings', () => {
  beforeEach(() => {
    mockedApiFetch.mockReset()
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('loads and displays the current site_name', async () => {
    mockedApiFetch.mockResolvedValueOnce({ site_name: 'My Site' })
    renderWithQueryClient()

    expect(await screen.findByLabelText('サイト名')).toHaveValue('My Site')
  })

  it('updates the site_name and shows a success toast', async () => {
    mockedApiFetch.mockResolvedValueOnce({ site_name: 'My Site' })
    mockedApiFetch.mockResolvedValueOnce({ site_name: 'New Site' })
    const user = userEvent.setup()
    renderWithQueryClient()

    const input = await screen.findByLabelText('サイト名')
    await user.clear(input)
    await user.type(input, 'New Site')
    await user.click(screen.getByRole('button', { name: '保存' }))

    expect(await screen.findByText('保存しました。')).toBeInTheDocument()
    expect(mockedApiFetch).toHaveBeenLastCalledWith('/api/settings', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ site_name: 'New Site' }),
    })
  })

  it('shows an inline error for a client-side out-of-range site_name without calling the API', async () => {
    mockedApiFetch.mockResolvedValueOnce({ site_name: 'My Site' })
    const user = userEvent.setup()
    renderWithQueryClient()

    const input = await screen.findByLabelText('サイト名')
    await user.clear(input)
    await user.click(screen.getByRole('button', { name: '保存' }))

    expect(await screen.findByText('サイト名は1〜100文字で入力してください。')).toBeInTheDocument()
    expect(mockedApiFetch).toHaveBeenCalledTimes(1)
  })

  it('maps a 400 "invalid site_name" response from the server to an inline error', async () => {
    mockedApiFetch.mockResolvedValueOnce({ site_name: 'My Site' })
    mockedApiFetch.mockRejectedValueOnce(new ApiError(400, 'invalid site_name'))
    const user = userEvent.setup()
    renderWithQueryClient()

    const input = await screen.findByLabelText('サイト名')
    await user.clear(input)
    await user.type(input, 'Valid Name')
    await user.click(screen.getByRole('button', { name: '保存' }))

    expect(await screen.findByText('サイト名は1〜100文字で入力してください。')).toBeInTheDocument()
    expect(mockedApiFetch).toHaveBeenCalledTimes(2)
  })
})
