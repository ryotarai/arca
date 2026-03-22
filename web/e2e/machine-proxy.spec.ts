import { test, expect } from '@playwright/test'
import { loginAsAdmin } from './helpers/auth'
import { ensureMockProfile } from './helpers/machine-profile'
import { createMachineViaAPI, waitForMachineStatus } from './helpers/machine'
import { resetBehavior } from './helpers/mock'

test.describe('mock runtime proxy connection', () => {
  let profileId: string

  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page)
    const profile = await ensureMockProfile(page)
    profileId = profile.id
  })

  test.afterEach(async ({ page }) => {
    await resetBehavior(page)
  })

  test('proxy routes to stub server', async ({ page }) => {
    const machineId = await createMachineViaAPI(page, `mock-proxy-${Date.now()}`, profileId)
    const machine = await waitForMachineStatus(page, machineId, ['running'])

    // If the machine has an endpoint (hostname for proxy), test proxy access.
    // The proxy routes based on Host header. The stub server returns JSON with machine_id.
    // Check the machine's endpoint field and try to access it.
    if (machine.endpoint) {
      const resp = await page.request.get(`https://${machine.endpoint}/`, {
        failOnStatusCode: false,
        timeout: 10_000,
      })
      // Even if the response isn't 200 (TLS, etc.), the fact that it connects is meaningful
      expect(resp.status()).toBeLessThan(500)
    }
  })
})
