import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// The Go server proxies nothing: in dev, Vite forwards the API to it. The server accepts the
// 127.0.0.1:5173 origin only when it was started with --dev.
const backend = 'http://127.0.0.1:7420'

export default defineConfig({
  plugins: [react()],
  server: {
    host: '127.0.0.1',
    port: 5173,
    strictPort: true,
    proxy: { '/api': backend, '/stream': backend },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    // The server's Content-Security-Policy allows only 'self', so nothing may be inlined as a data: URI.
    assetsInlineLimit: 0,
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    env: { VITE_MOCK: '1' },
    // Off, except tokens.css, which theme.test.ts reads as text.
    css: { include: [/styles\/tokens\.css/] },
  },
})
