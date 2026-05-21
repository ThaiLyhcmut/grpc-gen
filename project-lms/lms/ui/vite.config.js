import react from '@vitejs/plugin-react'

// The UI calls `/query` (relative); Vite proxies it to the gateway so the
// browser sees a same-origin request — no CORS, no gateway change needed.
export default {
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/query': { target: 'http://localhost:8080', changeOrigin: true },
    },
  },
}
