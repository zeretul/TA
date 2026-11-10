import { defineConfig } from '@playwright/test'
const port = process.env.TA_FANTASY_TEST_PORT || '15193'
export default defineConfig({
  testDir: './tests', testMatch: 'fantasy-world.spec.ts', workers: 1,
  outputDir: './test-results/fantasy-world', reporter: [['list'], ['json', { outputFile: './test-results/fantasy-world.json' }]],
  use: { baseURL: `http://127.0.0.1:${port}`, launchOptions: { executablePath: process.env.TA_CHROMIUM_EXECUTABLE_PATH }, contextOptions: { reducedMotion: 'reduce' }, screenshot: 'only-on-failure', trace: 'retain-on-failure' },
  webServer: { command: `node node_modules/vite/bin/vite.js --config playwright.fantasy.vite.config.ts --host 127.0.0.1 --port ${port} --strictPort`, url: `http://127.0.0.1:${port}/tests/fixtures/fantasy-world.html`, reuseExistingServer: false, env: { TA_TEST_VITE_CACHE_DIR: '.vite-fantasy' } },
})
