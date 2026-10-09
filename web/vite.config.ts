import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// In dev the panel API runs on :8000; the UI proxies to it.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:8000',
      '/sub': {
        target: 'http://127.0.0.1:8000',
        // let the SPA render /sub/<token> pages for browser navigations
        bypass: (req) => (req.headers.accept?.includes('text/html') ? req.url : undefined),
      },
    },
  },
  build: { outDir: 'dist', emptyOutDir: true, chunkSizeWarningLimit: 800 },
})
