import { expect, test } from '@playwright/test'
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

    const machineName = `critical-${Date.now().toString(36)}`
    let machineID = ''

    try {
      await page.getByRole('link', { name: 'Machines' }).click()
      await expect(page).toHaveURL('/machines')

      await page.getByLabel('Name').fill(machineName)
      await page.getByRole('button', { name: 'Create' }).click()
      await expect(page.locator('p.font-medium', { hasText: machineName })).toBeVisible()

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

      await page.goto(`/machines/${machineID}`)
      await expect(page.getByRole('heading', { name: 'Machine detail' })).toBeVisible()
      await expect(page.getByText('running', { exact: false })).toBeVisible()

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
