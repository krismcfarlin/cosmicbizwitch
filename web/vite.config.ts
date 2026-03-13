import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/logs': { target: 'http://localhost:8085', changeOrigin: true },
      '/api':  { target: 'http://localhost:8085', changeOrigin: true },
      '/health': { target: 'http://localhost:8085', changeOrigin: true },
    },
  },
  build: {
    outDir: '../internal/ui/dist',
    emptyOutDir: true,
  },
})
