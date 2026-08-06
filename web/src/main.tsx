// 参照: docs/plans/admin-spa/design.md §5.2 (認証切れフロー)

import React from 'react'
import ReactDOM from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { configureApiClient } from './api/client'
import { router } from './router'

const queryClient = new QueryClient()

// design.md §5.2: apiFetch が401を検知したら queryClient.clear() 後に
// /admin/login へ、現在のパスを redirect search パラメータとして渡してナビゲートする。
configureApiClient({
  queryClient,
  onUnauthorized: () => {
    const redirect = router.state.location.href
    void router.navigate({ to: '/login', search: { redirect } })
  },
})

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </React.StrictMode>,
)
