import { realpathSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const projectRoot = realpathSync(fileURLToPath(new URL('.', import.meta.url)))

export default defineConfig({
  root: projectRoot,
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:7891',
        changeOrigin: true,
      },
      '/api-token': {
        target: 'http://127.0.0.1:7891',
        changeOrigin: true,
      },
      '/onboarding': {
        target: 'http://127.0.0.1:7891',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) {
            return undefined
          }
          if (id.includes('@tanstack/react-query')) {
            return 'query-vendor'
          }
          if (id.includes('react-i18next') || id.includes('i18next')) {
            return 'i18n-vendor'
          }
          return 'vendor'
        },
      },
    },
  },
})
