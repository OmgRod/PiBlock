import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    // default dev server options; CLI flags (--host, --port) will override these
    host: true,
    port: 5173,
    // proxy API calls to the local Go internal API so frontend code can use
    // same-origin fetch('/lists') etc. when running the Vite dev server.
    proxy: {
      // proxy top-level API routes the app expects
      '/lists': {
        target: 'http://127.0.0.1:8081',
        changeOrigin: true,
        rewrite: (path) => path
      },
      '/analytics': { target: 'http://127.0.0.1:8081', changeOrigin: true },
      '/validate': { target: 'http://127.0.0.1:8081', changeOrigin: true },
      '/reload': { target: 'http://127.0.0.1:8081', changeOrigin: true },
      '/logs': { target: 'http://127.0.0.1:8081', changeOrigin: true },
      // legacy /api prefix
      '/api': { target: 'http://127.0.0.1:8081', changeOrigin: true }
    }
  }
})
