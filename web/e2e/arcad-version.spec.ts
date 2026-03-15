import { expect, test } from '@playwright/test'
import { loginAsAdmin } from './helpers/auth'
import {
  bestEffortDeleteMachine,
  createMachineWithTokenViaAPI,
} from './helpers/machine'
import { ensureLxdRuntime } from './helpers/runtime'

test.describe('arcad version', () => {
  test('GET /arcad/version returns version with valid machine token', async ({ page }) => {
    await loginAsAdmin(page)
    const runtime = await ensureLxdRuntime(page)
    const machine = await createMachineWithTokenViaAPI(page, `ver-${Date.now()}`, runtime.id)

    try {
      const response = await page.request.get('/arcad/version', {
        headers: { Authorization: `Bearer ${machine.machineToken}` },
      })
      expect(response.status()).toBe(200)
      const body = await response.text()
      expect(body.length).toBeGreaterThan(0)
    } finally {
      await bestEffortDeleteMachine(page, machine.id)
    }
  })

  test('GET /arcad/version rejects request without token', async ({ page }) => {
    await loginAsAdmin(page)

    const response = await page.request.get('/arcad/version', {
      failOnStatusCode: false,
    })
    expect(response.status()).toBe(401)
  })

  test('ReportMachineReadiness stores arcad_version', async ({ page }) => {
    await loginAsAdmin(page)
    const runtime = await ensureLxdRuntime(page)
    const machine = await createMachineWithTokenViaAPI(page, `rpt-${Date.now()}`, runtime.id)

    try {
      // Report readiness with arcad_version.
      const reportResp = await page.request.post(
        '/arca.v1.TunnelService/ReportMachineReadiness',
        {
          headers: { Authorization: `Bearer ${machine.machineToken}` },
          data: {
            ready: true,
            reason: 'e2e test',
            machineId: machine.id,
            arcadVersion: 'v0.99.0-e2e',
          },
        },
      )
      expect(reportResp.ok()).toBeTruthy()
      const reportPayload = (await reportResp.json()) as { accepted?: boolean }
      expect(reportPayload.accepted).toBe(true)

      // Verify version is stored by fetching machine details.
      const getResp = await page.request.post('/arca.v1.MachineService/GetMachine', {
        data: { machineId: machine.id },
      })
      expect(getResp.ok()).toBeTruthy()
      const getPayload = (await getResp.json()) as {
        machine?: { arcadVersion?: string }
      }
      expect(getPayload.machine?.arcadVersion).toBe('v0.99.0-e2e')
    } finally {
      await bestEffortDeleteMachine(page, machine.id)
    }
  })

  test('ReportMachineReadiness without arcad_version preserves existing', async ({ page }) => {
    await loginAsAdmin(page)
    const runtime = await ensureLxdRuntime(page)
    const machine = await createMachineWithTokenViaAPI(page, `compat-${Date.now()}`, runtime.id)

    try {
      // First report with version.
      await page.request.post('/arca.v1.TunnelService/ReportMachineReadiness', {
        headers: { Authorization: `Bearer ${machine.machineToken}` },
        data: {
          ready: true,
          reason: 'e2e first',
          machineId: machine.id,
          arcadVersion: 'v1.0.0',
        },
      })

      // Second report without version (backward compat).
      await page.request.post('/arca.v1.TunnelService/ReportMachineReadiness', {
        headers: { Authorization: `Bearer ${machine.machineToken}` },
        data: {
          ready: true,
          reason: 'e2e second',
          machineId: machine.id,
        },
      })

      // Version should still be v1.0.0.
      const getResp = await page.request.post('/arca.v1.MachineService/GetMachine', {
        data: { machineId: machine.id },
      })
      expect(getResp.ok()).toBeTruthy()
      const getPayload = (await getResp.json()) as {
        machine?: { arcadVersion?: string }
      }
      expect(getPayload.machine?.arcadVersion).toBe('v1.0.0')
    } finally {
      await bestEffortDeleteMachine(page, machine.id)
    }
  })
})
