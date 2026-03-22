import { defineConfig } from '@playwright/test'

export default defineConfig({
  globalSetup: './e2e/global-setup.ts',
  globalTeardown: './e2e/global-teardown.ts',
  workers: 1,
  testDir: './e2e',
  timeout: 30_000,
  expect: {
    timeout: 5_000,
  },
  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL ?? 'http://127.0.0.1:18080',
    headless: true,
  },
  projects: [
    {
      name: 'fast',
      testIgnore: /(?:lxd-provisioning|critical-user-journey)\.spec\.ts$/,
      timeout: 60_000,
    },
    {
      name: 'slow',
      testMatch: /(?:lxd-provisioning|critical-user-journey)\.spec\.ts$/,
      timeout: 600_000,
    },
  ],
  webServer: {
    command:
      'cd .. && mkdir -p bin .cache/go-build .cache/go-mod && GOCACHE=$(pwd)/.cache/go-build GOMODCACHE=$(pwd)/.cache/go-mod go build -o ./bin/server ./cmd/server && rm -f /tmp/arca-e2e.db && ./bin/server',
    env: {
      ...process.env,
      SERVER_ADDR: '0.0.0.0:18080',
      DB_DSN: 'file:/tmp/arca-e2e.db?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)',
      ARCA_SKIP_SETUP: '1',
      ARCA_ENABLE_MOCK: 'true',
      ARCA_ENCRYPTION_KEY: process.env.ARCA_ENCRYPTION_KEY ?? '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef',
      ARCA_WORKER_POLL_INTERVAL: '10s',
      ARCA_WORKER_CONCURRENCY: '1',
    },
    url: process.env.PLAYWRIGHT_BASE_URL ?? 'http://127.0.0.1:18080/',
    reuseExistingServer: true,
    timeout: 120_000,
  },
})
