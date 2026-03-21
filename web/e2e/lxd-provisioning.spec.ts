import { expect, test } from '@playwright/test'
import { loginAsAdmin, defaultSetupConfig } from './helpers/auth'
import {
  bestEffortDeleteMachine,
  createMachineViaAPI,
  waitForMachineStatus,
} from './helpers/machine'
import { ensureLxdTemplateWithProxyExposure } from './helpers/machine-template'

test.describe('LXD provisioning (proxy via server)', () => {
  let templateId = ''

  test.beforeAll(async ({ browser }) => {
    const context = await browser.newContext({
      baseURL: process.env.PLAYWRIGHT_BASE_URL ?? 'http://127.0.0.1:18080',
    })
    const page = await context.newPage()
    try {
      await loginAsAdmin(page)
      const runtime = await ensureLxdTemplateWithProxyExposure(page, {
        name: 'lxd-provisioning-e2e',
      })
      templateId = runtime.id
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
      machineID = await createMachineViaAPI(page, machineName, templateId)

      // Wait for running status (up to 10 minutes)
      const runningMachine = await waitForMachineStatus(page, machineID, ['running'], {
        timeoutMs: 10 * 60 * 1000,
        intervalMs: 5_000,
      })

      // Compute machine endpoint from setup state (baseDomain + domainPrefix + machineName)
      const machineEndpoint = `${defaultSetupConfig.domainPrefix}${machineName}.${defaultSetupConfig.baseDomain}`
      const serverBaseURL = process.env.PLAYWRIGHT_BASE_URL ?? 'http://127.0.0.1:18080'
      const ttydStatus = await accessTtydViaProxy(page, serverBaseURL, machineEndpoint)
      expect(ttydStatus).toBe(200)

      // Verify UI shows running status and endpoint
      await page.goto(`/machines/${machineID}`)
      await expect(page.getByRole('heading', { name: machineName })).toBeVisible()
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
      await page.getByRole('button', { name: 'Delete machine' }).click()
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

/**
 * Walk through the full arcad auth flow to access ttyd via proxy-via-server.
 *
 * The machine hostname (e.g. arca-xxx.localhost) doesn't resolve in the test
 * environment, so we manually follow each redirect step, rewriting machine
 * hostname URLs to server URL + Host header.
 *
 * Flow:
 *  1. GET /__arca/ttyd/ (Host: endpoint) → 302 to /console/authorize?target=...
 *  2. GET /console/authorize?target=... (server session) → 302 to callback URL
 *  3. GET /callback?token=... (Host: endpoint) → 302 + Set-Cookie: arcad_session
 *  4. GET /__arca/ttyd/ (Host: endpoint + arcad_session cookie) → 200
 */
async function accessTtydViaProxy(
  page: import('@playwright/test').Page,
  serverBaseURL: string,
  machineEndpoint: string,
): Promise<number> {
  // Step 1: Initial request to ttyd → arcad redirects to authorize
  const step1 = await page.request.get(`${serverBaseURL}/__arca/ttyd/`, {
    headers: { Host: machineEndpoint },
    failOnStatusCode: false,
    maxRedirects: 0,
    timeout: 20_000,
  })
  if (step1.status() !== 302) {
    return step1.status()
  }

  const authorizeLocation = step1.headers()['location'] ?? ''
  if (!authorizeLocation.includes('/console/authorize')) {
    throw new Error(`Expected authorize redirect, got: ${authorizeLocation}`)
  }

  // Step 2: Follow authorize redirect on the server (page has admin session)
  // The Location is an absolute URL pointing to the machine hostname's authorize
  // endpoint, but the authorize handler is on the arca server itself. Rewrite
  // to hit the server directly.
  const authorizeURL = rewriteToServer(authorizeLocation, serverBaseURL)
  const step2 = await page.request.get(authorizeURL, {
    failOnStatusCode: false,
    maxRedirects: 0,
    timeout: 20_000,
  })
  if (step2.status() !== 302) {
    throw new Error(`Authorize returned ${step2.status()}, expected 302`)
  }

  const callbackLocation = step2.headers()['location'] ?? ''
  if (!callbackLocation.includes('callback') || !callbackLocation.includes('token=')) {
    throw new Error(`Expected callback redirect with token, got: ${callbackLocation}`)
  }

  // Step 3: Follow callback redirect → arcad exchanges token for session cookie
  // Rewrite machine hostname URL to server URL + Host header
  const callbackURL = rewriteToServer(callbackLocation, serverBaseURL)
  const step3 = await page.request.get(callbackURL, {
    headers: { Host: machineEndpoint },
    failOnStatusCode: false,
    maxRedirects: 0,
    timeout: 20_000,
  })
  if (step3.status() !== 302) {
    throw new Error(`Callback returned ${step3.status()}, expected 302`)
  }

  // Extract arcad_session cookie from Set-Cookie header
  const setCookieHeader = step3.headers()['set-cookie'] ?? ''
  const sessionMatch = setCookieHeader.match(/arcad_session=([^;]+)/)
  if (!sessionMatch) {
    throw new Error(`No arcad_session cookie in callback response: ${setCookieHeader}`)
  }
  const arcadSession = sessionMatch[1]

  // Step 4: Final request to ttyd with arcad_session cookie → should get 200
  const step4 = await page.request.get(`${serverBaseURL}/__arca/ttyd/`, {
    headers: {
      Host: machineEndpoint,
      Cookie: `arcad_session=${arcadSession}`,
    },
    failOnStatusCode: false,
    maxRedirects: 0,
    timeout: 20_000,
  })

  return step4.status()
}

/**
 * Rewrite a URL that points to a machine hostname to point to the server instead.
 * E.g. http://arca-xxx.localhost:18080/callback?token=abc → http://127.0.0.1:18080/callback?token=abc
 */
function rewriteToServer(locationURL: string, serverBaseURL: string): string {
  try {
    const parsed = new URL(locationURL)
    const server = new URL(serverBaseURL)
    parsed.protocol = server.protocol
    parsed.hostname = server.hostname
    parsed.port = server.port
    return parsed.toString()
  } catch {
    // Relative URL — prepend server base
    return serverBaseURL.replace(/\/$/, '') + locationURL
  }
}
