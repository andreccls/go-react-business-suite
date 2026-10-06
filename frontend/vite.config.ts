/// <reference types="vitest/config" />
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// In dev the SPA calls /api on its own origin and Vite forwards it to the API (like nginx does in
// production), so no CORS is involved. API_PROXY_TARGET defaults to the host-published API port.
const target = process.env.API_PROXY_TARGET ?? 'http://localhost:8095'

export default defineConfig({
  plugins: [react()],
  server: {
    host: true,
    proxy: { '/api': { target, rewrite: (p) => p.replace(/^\/api/, '') } },
  },
  build: { sourcemap: false },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    env: { VITE_API_BASE: 'http://api.test/api', VITE_BUSINESS_TZ: 'America/Sao_Paulo' },
    css: false,
    coverage: {
      provider: 'v8',
      include: ['src/**/*.{ts,tsx}'],
      // Exclusions are justified in docs/TESTING.md.
      exclude: ['src/main.tsx', 'src/api/schema.d.ts', 'src/test/**', 'src/**/*.test.{ts,tsx}', 'src/vite-env.d.ts'],
      reporter: ['text', 'text-summary', 'json-summary'],
      thresholds: {
        lines: 90,
        branches: 90,
        functions: 90,
        statements: 90,
        'src/lib/**': { lines: 95, branches: 90, functions: 95, statements: 95 },
        'src/api/client.ts': { lines: 95, branches: 90, functions: 95, statements: 95 },
      },
    },
  },
})
