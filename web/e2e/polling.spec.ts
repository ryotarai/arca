import { expect, test, type Page, type Request } from '@playwright/test'

function isRequestFor(request: Request, path: string): boolean {
  return request.url().endsWith(path) && request.method() === 'POST'
}

async function ensureSetupCompleted(page: Page) {
  await page.goto('/')
  if (!page.url().endsWith('/setup')) {
    return
  }

  await page.getByLabel('Email').fill(`setup-${Date.now()}@example.com`)
  await page.getByLabel('Password').fill('password123')
  await page.getByLabel('Confirm password').fill('password123')
  await page.getByRole('button', { name: 'Continue' }).click()

  await page.getByLabel('Base domain').fill('example.test')
  await page.getByLabel('Cloudflare account ID').fill('test-account-id')
  await page.getByLabel('Cloudflare API token').fill('test-token')
  await page.getByRole('button', { name: 'Validate and continue' }).click()

  await page.getByRole('button', { name: 'Finish setup' }).click()
  await expect(page).toHaveURL('/')
}

async function registerAndLogin(page: Page) {
  const email = `polling-${Date.now()}@example.com`
  const password = 'password123'

  await page.goto('/login')
  await page.getByRole('button', { name: 'Create new account' }).click()
  await page.getByLabel('Email').fill(email)
  await page.getByLabel('Password').fill(password)
  await page.getByRole('button', { name: 'Register' }).click()

  await expect(page.getByText('registered. please log in.')).toBeVisible()
  await page.getByLabel('Password').fill(password)
  await page.getByRole('button', { name: 'Login' }).click()
  await expect(page).toHaveURL('/')
}

function trackMaxConcurrentRequests(page: Page, path: string): () => number {
  let inFlight = 0
  let maxInFlight = 0

  const onStart = (request: Request) => {
    if (!isRequestFor(request, path)) {
      return
    }
    inFlight += 1
    maxInFlight = Math.max(maxInFlight, inFlight)
  }

  const onDone = (request: Request) => {
    if (!isRequestFor(request, path)) {
      return
    }
    inFlight -= 1
  }

  page.on('request', onStart)
  page.on('requestfinished', onDone)
  page.on('requestfailed', onDone)

  return () => {
    page.off('request', onStart)
    page.off('requestfinished', onDone)
    page.off('requestfailed', onDone)
    return maxInFlight
  }
}

test('machines list polling does not overlap in-flight requests', async ({ page }) => {
  await ensureSetupCompleted(page)
  await registerAndLogin(page)

  const endpoint = '/arca.v1.MachineService/ListMachines'
  let firstRequest = true

  await page.route(`**${endpoint}`, async (route) => {
    if (firstRequest) {
      firstRequest = false
      await new Promise((resolve) => setTimeout(resolve, 6500))
    }
    await route.continue()
  })

  const stopTracking = trackMaxConcurrentRequests(page, endpoint)
  await page.goto('/machines')

  await page.waitForRequest((request) => isRequestFor(request, endpoint))
  await page.waitForTimeout(4000)

  const maxInFlight = stopTracking()
  expect(maxInFlight).toBe(1)
  await page.unrouteAll({ behavior: 'ignoreErrors' })
})

test('machine detail polling does not overlap in-flight requests', async ({ page }) => {
  await ensureSetupCompleted(page)
  await registerAndLogin(page)

  await page.goto('/machines')
  await page.getByLabel('Name').fill('polling-machine')
  await page.getByRole('button', { name: 'Create' }).click()
  await expect(page.locator('p.font-medium', { hasText: /^polling-machine$/ })).toBeVisible()

  const endpoint = '/arca.v1.MachineService/GetMachine'
  let firstRequest = true

  await page.route(`**${endpoint}`, async (route) => {
    if (firstRequest) {
      firstRequest = false
      await new Promise((resolve) => setTimeout(resolve, 6500))
    }
    await route.continue()
  })

  const stopTracking = trackMaxConcurrentRequests(page, endpoint)
  await page.getByRole('link', { name: 'Details' }).first().click()

  await page.waitForRequest((request) => isRequestFor(request, endpoint))
  await page.waitForTimeout(4000)

  const maxInFlight = stopTracking()
  expect(maxInFlight).toBe(1)
  await page.unrouteAll({ behavior: 'ignoreErrors' })
})
