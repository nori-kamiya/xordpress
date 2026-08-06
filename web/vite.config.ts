import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  base: '/admin/', // /assets/ 配下は公開テーマの静的配信と衝突するため必須
  plugins: [react()],
  build: {
    outDir: '../internal/adminui/dist',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8099', // 開発用バックエンドの --base-url と一致させる
        changeOrigin: true,
        configure: (proxy) => {
          proxy.on('proxyReq', (proxyReq) => {
            // ブラウザが送るOriginをそのまま転送するとCSRFMiddlewareの
            // 文字列完全一致チェックに落ちるため、バックエンドの
            // base-urlに書き換える。
            proxyReq.setHeader('Origin', 'http://127.0.0.1:8099')
          })
        },
      },
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/setupTests.ts'],
  },
})
