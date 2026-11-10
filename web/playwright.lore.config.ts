import { defineConfig } from '@playwright/test'

const port = process.env.TA_LORE_TEST_PORT || '15179'
export default defineConfig({
  testDir: './tests', testMatch: 'lore-social.spec.ts', workers: 1,
  outputDir: './test-results/lore-social', reporter: [['list']],
  use: { baseURL: `http://127.0.0.1:${port}`, launchOptions: { executablePath: process.env.TA_CHROMIUM_EXECUTABLE_PATH }, screenshot: 'only-on-failure', trace: 'retain-on-failure' },
  webServer: { command: `pnpm dev --host 127.0.0.1 --port ${port} --strictPort`, url: `http://127.0.0.1:${port}/tests/fixtures/lore-social.html`, reuseExistingServer: false,
    env: { TA_TEST_VITE_CACHE_DIR: `node_modules/.vite-lore-${port}` } },
})
