import { test } from '@playwright/test'
import { loginAsAdmin } from './helpers/auth'
import { ensureMockProfile } from './helpers/machine-profile'
import { createMachineViaAPI, waitForMachineStatus } from './helpers/machine'
import { setMachineBehavior, resetBehavior } from './helpers/mock'

test.describe('mock runtime error injection', () => {
  let profileId: string

  test.beforeEach(async ({ page }) => {
    await loginAsAdmin(page)
    const profile = await ensureMockProfile(page)
    profileId = profile.id
  })

  test.afterEach(async ({ page }) => {
    await resetBehavior(page)
  })

  test('machine fails to start with injected error', async ({ page }) => {
    const machineId = await createMachineViaAPI(page, `mock-err-${Date.now()}`, profileId)
    await setMachineBehavior(page, machineId, {
      errorOn: { EnsureRunning: 'simulated disk full' },
    })
    await waitForMachineStatus(page, machineId, ['failed'], { timeoutMs: 60_000 })
  })
})
