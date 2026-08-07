// 参照: docs/plans/admin-spa/design.md §2 (ファイル構成)・§5.2 (認証切れフロー)・§9 (basepath制約)
//
// TanStack Router のルートツリーをコードで明示的に組み立てる(ファイルベースルーティングは使わない)。
// T6〜T11 が各画面を実装するまでは空のプレースホルダコンポーネントを登録し、
// tsc とルーティングの型(ToOptions等)が通ることだけを保証する。

import { createRootRoute, createRoute, createRouter } from '@tanstack/react-router'
import Root from './routes/root'
import Setup from './routes/setup'
import Login from './routes/login'
import PostsIndex from './routes/posts.index'
import PostsNew from './routes/posts.new'
import PostsEdit from './routes/posts.edit'
import Settings from './routes/settings'
import AccountSessions from './routes/account.sessions'

const rootRoute = createRootRoute({
  component: Root,
})

const setupRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/setup',
  component: Setup,
})

interface LoginSearch {
  redirect?: string
  /** setup=done — セットアップ完了直後のログイン画面遷移時に成功バナーを出すためのフラグ (design.md §5.1) */
  setup?: string
}

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/login',
  validateSearch: (search: Record<string, unknown>): LoginSearch => ({
    redirect: typeof search.redirect === 'string' ? search.redirect : undefined,
    setup: typeof search.setup === 'string' ? search.setup : undefined,
  }),
  component: Login,
})

const postsIndexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/posts',
  component: PostsIndex,
})

const postsNewRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/posts/new',
  component: PostsNew,
})

const postsEditRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/posts/$postId/edit',
  component: PostsEdit,
})

const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings',
  component: Settings,
})

const accountSessionsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/account/sessions',
  component: AccountSessions,
})

const routeTree = rootRoute.addChildren([
  setupRoute,
  loginRoute,
  postsIndexRoute,
  postsNewRoute,
  postsEditRoute,
  settingsRoute,
  accountSessionsRoute,
])

// design.md §9: vite.config.ts の base:'/admin/' と対になる制約。
// TanStack Router内部のパス解決をこのプレフィックス配下に固定する。
export const router = createRouter({
  basepath: '/admin',
  routeTree,
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
