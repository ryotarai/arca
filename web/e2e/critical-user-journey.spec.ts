import { expect, test } from '@playwright/test'
import { loginAsAdmin } from './helpers/auth'
import {
  bestEffortDeleteMachine,
  waitForMachineByName,
  waitForMachineStatus,
  waitForTTYDAccess,
} from './helpers/machine'
import { ensureLxdProfile } from './helpers/machine-profile'

test.describe('critical user journey', () => {
  test('login -> create machine -> running readiness -> ttyd is reachable', async ({ page }) => {
    await loginAsAdmin(page)
    await ensureLxdProfile(page)

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
      await expect(page.getByRole('heading', { name: machineName })).toBeVisible()

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
      await expect(page.getByRole('heading', { name: machineName })).toBeVisible()
      await expect(page.getByText('running', { exact: false })).toBeVisible()
      await expect(page.getByRole('button', { name: 'Delete machine' })).toBeVisible()

      await page.getByRole('link', { name: 'Machine Profiles' }).click()
      await expect(page).toHaveURL('/machine-profiles')
      await expect(page.getByPlaceholder('edge-libvirt')).toHaveCount(0)
      await expect(page.getByPlaceholder('main-libvirt')).toBeVisible()

      await page.getByRole('link', { name: 'Admin settings' }).click()
      await expect(page).toHaveURL('/admin/settings')

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
