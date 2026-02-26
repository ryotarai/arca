import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  timeout: 30_000,
  expect: {
    timeout: 5_000,
  },
  use: {
    baseURL: 'http://127.0.0.1:18080',
    headless: true,
  },
  webServer: {
    command: 'cd .. && make build-server && SERVER_ADDR=127.0.0.1:18080 ./bin/server',
    url: 'http://127.0.0.1:18080/api/health',
    reuseExistingServer: true,
    timeout: 120_000,
  },
})
