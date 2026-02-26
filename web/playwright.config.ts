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
    command:
      'cd .. && mkdir -p bin .cache/go-build .cache/go-mod && GOCACHE=$(pwd)/.cache/go-build GOMODCACHE=$(pwd)/.cache/go-mod go build -o ./bin/server ./cmd/server && SERVER_ADDR=127.0.0.1:18080 ./bin/server',
    url: 'http://127.0.0.1:18080/',
    reuseExistingServer: true,
    timeout: 120_000,
  },
})
