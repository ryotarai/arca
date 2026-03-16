import { expect, test } from '@playwright/test'
import { loginAsAdmin } from './helpers/auth'
import {
  bestEffortDeleteMachine,
  createMachineViaAPI,
  waitForMachineStatus,
} from './helpers/machine'
import { ensureLxdRuntimeWithProxyExposure } from './helpers/runtime'

test.describe('LXD provisioning (proxy via server)', () => {
  let runtimeId = ''

  test.beforeAll(async ({ browser }) => {
    const context = await browser.newContext({
      baseURL: process.env.PLAYWRIGHT_BASE_URL ?? 'http://127.0.0.1:18080',
    })
    const page = await context.newPage()
    try {
      await loginAsAdmin(page)
      const runtime = await ensureLxdRuntimeWithProxyExposure(page, {
        name: 'lxd-provisioning-e2e',
        domainPrefix: 'arca-',
        baseDomain: 'localhost',
      })
      runtimeId = runtime.id
    } finally {
      await context.close()
    }
  })

  test('full machine lifecycle: create -> running -> stop -> start -> delete', async ({
    page,
  }) => {
    test.setTimeout(600_000)
    await loginAsAdmin(page)
    const machineName = `lxd-e2e-${Date.now().toString(36)}`
    let machineID = ''

    try {
      machineID = await createMachineViaAPI(page, machineName, runtimeId)

      // Wait for running status (up to 10 minutes)
      const runningMachine = await waitForMachineStatus(page, machineID, ['running'], {
        timeoutMs: 10 * 60 * 1000,
        intervalMs: 5_000,
      })

      // Verify endpoint is set
      expect(runningMachine.endpoint?.trim() ?? '').not.toEqual('')

      // Test proxy via server access: send request to server with Host header.
      // The request may return 200 (authenticated), 302 (auth redirect), or
      // 401/403 (rejected). Any of these confirms the proxy is routing correctly.
      const machineEndpoint = runningMachine.endpoint!.trim()
      const serverBaseURL = process.env.PLAYWRIGHT_BASE_URL ?? 'http://127.0.0.1:18080'
      const resp = await page.request.get(`${serverBaseURL}/__arca/ttyd/`, {
        headers: { Host: machineEndpoint },
        failOnStatusCode: false,
        maxRedirects: 0,
        timeout: 20_000,
      })
      expect([200, 302, 401, 403]).toContain(resp.status())

      // Verify UI shows running status and endpoint
      await page.goto(`/machines/${machineID}`)
      await expect(page.getByRole('heading', { name: 'Machine detail' })).toBeVisible()
      await expect(page.getByText('running')).toBeVisible()

      // Stop the machine
      page.once('dialog', (dialog) => dialog.accept())
      await page.getByRole('button', { name: 'Stop' }).click()

      await waitForMachineStatus(page, machineID, ['stopped'], {
        timeoutMs: 3 * 60 * 1000,
        intervalMs: 5_000,
      })

      // Start the machine again
      await page.goto(`/machines/${machineID}`)
      await page.getByRole('button', { name: 'Start', exact: true }).click()

      await waitForMachineStatus(page, machineID, ['running'], {
        timeoutMs: 10 * 60 * 1000,
        intervalMs: 5_000,
      })

      // Delete the machine
      await page.goto(`/machines/${machineID}`)
      page.once('dialog', (dialog) => dialog.accept())
      await page.getByRole('button', { name: 'Delete' }).click()
      await expect(page).toHaveURL('/machines')

      // Verify machine is gone from list (deletion is async, poll until removed)
      await expect(async () => {
        await page.goto('/machines')
        await expect(page.locator('li', { hasText: machineName })).toHaveCount(0)
      }).toPass({ timeout: 60_000 })
      machineID = '' // Already deleted
    } finally {
      if (machineID !== '') {
        await bestEffortDeleteMachine(page, machineID)
      }
    }
  })
})
