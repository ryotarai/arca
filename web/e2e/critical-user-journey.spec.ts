import { expect, test } from '@playwright/test'
import { execFileSync } from 'node:child_process'
import {
  bestEffortDeleteMachine,
  cloudflareIntegrationConfig,
  loginAsAdmin,
  skipCloudflareIntegrationIfMissing,
  validateCloudflareToken,
  waitForMachineByName,
  waitForMachineStatus,
  waitForTTYDAccess,
} from './helpers'

const e2eDBPath = '/tmp/arca-e2e.db'

function ensureLxdRuntimeInDB() {
  execFileSync('sqlite3', [
    e2eDBPath,
    `INSERT OR IGNORE INTO runtimes (id, name, type, config_json, created_at, updated_at) VALUES ('lxd', 'lxd-default', 'lxd', '{"lxd":{"endpoint":"https://localhost:8443"}}', CAST(strftime('%s','now') AS INTEGER), CAST(strftime('%s','now') AS INTEGER));`,
  ], { stdio: 'pipe' })
}

test.describe('critical user journey', () => {
  skipCloudflareIntegrationIfMissing()

  test('login -> create machine -> running readiness -> ttyd is reachable', async ({ page }) => {
    const configResult = cloudflareIntegrationConfig()
    if (configResult.config == null) {
      test.fail(true, 'Cloudflare config should exist when this test is not skipped')
      return
    }

    await validateCloudflareToken(page, configResult.config)
    await loginAsAdmin(page)
    ensureLxdRuntimeInDB()

    const machineName = `critical-${Date.now().toString(36)}`
    let machineID = ''

    try {
      await page.getByRole('link', { name: 'Machines' }).click()
      await expect(page).toHaveURL('/machines')

      await page.getByRole('link', { name: 'Create machine' }).click()
      await expect(page).toHaveURL('/machines/create')
      await page.getByLabel('Name').fill(machineName)
      await page.getByRole('button', { name: 'Create machine' }).click()
      await expect(page).toHaveURL(/\/machines\/.+/)
      await expect(page.getByRole('heading', { name: 'Machine detail' })).toBeVisible()

      const createdMachine = await waitForMachineByName(page, machineName, {
        timeoutMs: 90_000,
        intervalMs: 2_000,
      })
      machineID = createdMachine.id

      const runningMachine = await waitForMachineStatus(page, machineID, ['running'], {
        timeoutMs: 12 * 60 * 1000,
        intervalMs: 5_000,
      })

      expect(runningMachine.endpoint?.trim() ?? '').not.toEqual('')

      await page.goto('/machines')
      await expect(page).toHaveURL('/machines')
      const machineRow = page.locator('li', { hasText: machineName })
      await expect(machineRow.getByRole('button', { name: 'Start' })).toHaveCount(0)
      await expect(machineRow.getByRole('button', { name: 'Stop' })).toHaveCount(0)
      await expect(machineRow.getByRole('button', { name: 'Delete' })).toHaveCount(0)
      await machineRow.getByRole('link', { name: 'Details' }).click()

      await page.goto(`/machines/${machineID}`)
      await expect(page.getByRole('heading', { name: 'Machine detail' })).toBeVisible()
      await expect(page.getByText('running', { exact: false })).toBeVisible()
      await expect(page.getByRole('button', { name: 'Delete' })).toBeVisible()

      await page.getByRole('link', { name: 'Runtimes' }).click()
      await expect(page).toHaveURL('/runtimes')
      await expect(page.getByPlaceholder('edge-libvirt')).toHaveCount(0)
      await expect(page.getByPlaceholder('main-libvirt')).toBeVisible()

      await page.getByRole('link', { name: 'Settings' }).click()
      await expect(page).toHaveURL('/settings')
      await expect(page.getByText('Clear stored client secret on save')).toHaveCount(0)

      const endpoint = runningMachine.endpoint?.trim() ?? ''
      const ttydStatus = await waitForTTYDAccess(page, endpoint, {
        timeoutMs: 3 * 60 * 1000,
        intervalMs: 10_000,
      })
      expect([200, 401, 403]).toContain(ttydStatus)
    } finally {
      if (machineID !== '') {
        await bestEffortDeleteMachine(page, machineID)
      }
    }
  })
})
