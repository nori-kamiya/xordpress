// 参照: docs/plans/p1-multi-user-rbac/design.md §5・tasks.md T15
//
// root.tsx はナビゲーションの出し分け(CapUsersManageを持たない場合は
// 「ユーザー管理」リンクを隠す)と、/login・/setup でGET /api/accountを
// 呼ばないためのenabled:falseガードを担う。後者はuseLocation().pathnameが
// basepath('/admin')込みか剥がされた後かに依存するため、モックした
// useLocationではなく実際のrouter(createRouter({basepath:'/admin',...}))
// + createMemoryHistoryで検証する。

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryHistory, createRootRoute, createRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import Root from './root'
import { apiFetch } from '../api/client'
import type { CurrentUser } from '../api/account'

vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return {
    ...actual,
    apiFetch: vi.fn(),
  }
})

const mockedApiFetch = vi.mocked(apiFetch)

function StubPage() {
  return <div>stub page</div>
}

/** router.tsx と同じ basepath('/admin')・ルート構成を最小限で再現する。
 * router.tsx はシングルトンの router しかexportしないため、
 * initialEntriesを差し替えたいこのテストでは同等のルーターを別途組む。 */
function renderAt(pathname: string, currentUser: CurrentUser): void {
  const rootRoute = createRootRoute({ component: Root })
  const loginRoute = createRoute({ getParentRoute: () => rootRoute, path: '/login', component: StubPage })
  const postsRoute = createRoute({ getParentRoute: () => rootRoute, path: '/posts', component: StubPage })
  const routeTree = rootRoute.addChildren([loginRoute, postsRoute])

  const router = createRouter({
    basepath: '/admin',
    routeTree,
    history: createMemoryHistory({ initialEntries: [pathname] }),
  })

  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })

  mockedApiFetch.mockImplementation((path: string) => {
    if (path === '/api/account') {
      return Promise.resolve(currentUser)
    }
    return Promise.reject(new Error(`unexpected path: ${path}`))
  })

  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
}

const administrator: CurrentUser = {
  id: 1,
  username: 'admin',
  role: 'administrator',
  capabilities: ['users:manage', 'self:manage'],
}

const editor: CurrentUser = {
  id: 2,
  username: 'editor-1',
  role: 'editor',
  capabilities: ['self:manage'],
}

describe('Root', () => {
  beforeEach(() => {
    mockedApiFetch.mockReset()
  })

  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it('does not call GET /api/account on /login', async () => {
    renderAt('/admin/login', administrator)

    await screen.findByText('stub page')
    expect(mockedApiFetch).not.toHaveBeenCalledWith('/api/account')
  })

  it('shows the users management link for a user with users:manage', async () => {
    renderAt('/admin/posts', administrator)

    expect(await screen.findByRole('link', { name: 'ユーザー管理' })).toBeInTheDocument()
  })

  it('hides the users management link for a user without users:manage', async () => {
    renderAt('/admin/posts', editor)

    await screen.findByText('stub page')
    expect(screen.queryByRole('link', { name: 'ユーザー管理' })).not.toBeInTheDocument()
  })
})
