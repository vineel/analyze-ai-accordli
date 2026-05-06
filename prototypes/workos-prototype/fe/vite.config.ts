import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Proxy /api, /auth, /login, /logout to the Go backend so cookies stay
// same-origin during dev. Browser sees everything as localhost:5173.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api':    { target: 'http://localhost:8080', changeOrigin: false },
      '/auth':   { target: 'http://localhost:8080', changeOrigin: false },
      '/login':  { target: 'http://localhost:8080', changeOrigin: false },
      '/logout': { target: 'http://localhost:8080', changeOrigin: false },
    },
  },
})
