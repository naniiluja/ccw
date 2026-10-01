/// <reference types="vitest/config" />
import path from 'node:path'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig, type ProxyOptions } from 'vite'

// The Go server in be/ owns these paths. The SPA has routes with the same
// names (/accounts, /keys, ...), so in dev a browser navigation (Accept:
// text/html) is answered by the SPA and only fetch calls reach the server.
const upstream = 'http://127.0.0.1:20130'

const apiPaths = [
  '/api',
  '/login',
  '/logout',
  '/accounts',
  '/providers',
  '/provider-defs',
  '/keys',
  '/filters',
  '/quota',
  '/usage',
  '/errors',
  '/drift',
  '/oauth',
  '/ui-settings',
  '/account-tests',
  '/v1',
  '/mcp',
]

const proxyEntry: ProxyOptions = {
  target: upstream,
  changeOrigin: true,
  configure(proxy) {
    // guardRequest in the Go server compares Origin with Host on writes, so
    // the browser's dev-server origin has to become the upstream origin.
    proxy.on('proxyReq', (proxyReq) => {
      if (proxyReq.getHeader('origin')) proxyReq.setHeader('origin', upstream)
    })
  },
  bypass(req) {
    if (req.headers.accept?.includes('text/html')) return '/index.html'
  },
}

export default defineConfig(({ command }) => ({
  // The Go server serves the build under /ui/; dev serves from the root.
  base: command === 'build' ? '/ui/' : '/',
  plugins: [react(), tailwindcss()],
  resolve: { alias: { '@': path.resolve(import.meta.dirname, './src') } },
  server: {
    proxy: Object.fromEntries(apiPaths.map((p) => [p, proxyEntry])),
  },
  build: { outDir: 'dist', emptyOutDir: true, chunkSizeWarningLimit: 700 },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    css: false,
  },
}))
