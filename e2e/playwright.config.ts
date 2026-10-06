import { defineConfig, devices } from '@playwright/test'

// Runs inside the official Playwright image on the compose network: the SPA is http://frontend:8080.
export default defineConfig({
  testDir: './tests',
  workers: 1, // one shared database and a per-IP login rate limit: keep it sequential
  timeout: 60_000,
  expect: { timeout: 10_000 },
  reporter: [['list']],
  outputDir: '/tmp/playwright-output',
  use: {
    baseURL: process.env.E2E_BASE_URL ?? 'http://frontend:8080',
    locale: 'pt-BR',
    timezoneId: 'America/Sao_Paulo',
    trace: 'off',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
})
