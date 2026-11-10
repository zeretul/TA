import { defineConfig } from '@playwright/test'
const port = process.env.TA_PROGRESSION_TEST_PORT || '15183'
export default defineConfig({
  testDir: './tests', testMatch: 'adventure-progression.spec.ts', workers: 1,
  outputDir: './test-results/adventure-progression', reporter: [['list']],
  use: { baseURL: `http://127.0.0.1:${port}`, launchOptions: { executablePath: process.env.TA_CHROMIUM_EXECUTABLE_PATH }, contextOptions: { reducedMotion: 'reduce' }, screenshot: 'only-on-failure', trace: 'retain-on-failure' },
  webServer: { command: `pnpm dev --host 127.0.0.1 --port ${port} --strictPort`, url: `http://127.0.0.1:${port}/tests/fixtures/adventure-progression.html`, reuseExistingServer: false, env: { TA_TEST_VITE_CACHE_DIR: `node_modules/.vite-progression-${port}` } },
})
