import { expect, test, type Page, type APIResponse } from '@playwright/test'

const adminEmail = 'admin@example.com'
const adminPassword = 'password123'

async function parseJSONSafe(response: APIResponse): Promise<Record<string, unknown> | null> {
  try {
    return (await response.json()) as Record<string, unknown>
  } catch {
    return null
  }
}

async function ensureSetupAdmin(page: Page) {
  const response = await page.request.post('/arca.v1.SetupService/CompleteSetup', {
    data: {
      adminEmail,
      adminPassword,
      baseDomain: 'example.com',
      domainPrefix: 'arca-',
      cloudflareApiToken: 'dummy-token',
      cloudflareZoneId: 'dummy-zone',
      dockerProviderEnabled: false,
      machineRuntime: 'libvirt',
    },
  })

  if (response.ok()) {
    return
  }

  const payload = await parseJSONSafe(response)
  const code = String(payload?.code ?? '').toLowerCase()
  if (code === 'failed_precondition') {
    return
  }

  throw new Error(`setup failed: ${response.status()} ${JSON.stringify(payload)}`)
}

async function login(page: Page, email = adminEmail, password = adminPassword) {
  await ensureSetupAdmin(page)
  await page.goto('/login')
  await page.getByLabel('Email').fill(email)
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Login' }).click()
  await expect(page).toHaveURL('/')
}

test('redirect path exposes login screen', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('link', { name: 'Login' }).click()

  await expect(page).toHaveURL('/login')
  await expect(page.getByRole('heading', { name: 'Arca' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Login' })).toBeVisible()
})

test('login route is directly accessible', async ({ page }) => {
  await page.goto('/login')

  await expect(page).toHaveURL('/login')
  await expect(page.getByLabel('Email')).toBeVisible()
  await expect(page.getByLabel('Password', { exact: true })).toBeVisible()
})

test('unauthenticated user cannot access authenticated dashboard view', async ({ page }) => {
  await page.goto('/')

  await expect(page.getByRole('link', { name: 'Login' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Dashboard' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Logout' })).toHaveCount(0)
})

test('login and logout via Connect RPC', async ({ page }) => {
  await ensureSetupAdmin(page)

  await page.goto('/login')
  await page.getByLabel('Email').fill(adminEmail)
  await page.getByLabel('Password', { exact: true }).fill(adminPassword)

  const loginRequest = page.waitForRequest(
    (request) =>
      request.url().endsWith('/arca.v1.AuthService/Login') &&
      request.method() === 'POST',
  )
  await page.getByRole('button', { name: 'Login' }).click()
  await loginRequest

  await expect(page).toHaveURL('/')
  await expect(page.getByText(`Signed in as ${adminEmail}`)).toBeVisible()

  const logoutRequest = page.waitForRequest(
    (request) =>
      request.url().endsWith('/arca.v1.AuthService/Logout') &&
      request.method() === 'POST',
  )
  await page.getByRole('button', { name: 'Logout' }).click()
  await logoutRequest

  await expect(page.getByRole('link', { name: 'Login' })).toBeVisible()
})

test('machine CRUD screen works for authenticated user', async ({ page }) => {
  const machineName = `alpha-machine-${Date.now()}`
  await login(page)

  await page.getByRole('link', { name: 'Machines' }).click()
  await expect(page).toHaveURL('/machines')
  await expect(page.getByRole('heading', { name: 'Machines' })).toBeVisible()

  await page.getByLabel('Name').fill(machineName)
  await page.getByRole('button', { name: 'Create' }).click()
  await expect(page.locator('p.font-medium', { hasText: machineName })).toBeVisible()
  await expect(page.getByText(/pending|starting|running/).first()).toBeVisible()

  page.once('dialog', (dialog) => dialog.accept())
  await page.getByRole('button', { name: 'Stop' }).first().click()
  await expect(page.getByText('stopping').first()).toBeVisible()

  await page.getByRole('link', { name: 'Details' }).first().click()
  await expect(page).toHaveURL(/\/machines\/.+/)
  await expect(page.getByRole('heading', { name: 'Machine detail' })).toBeVisible()
  await expect(page.getByText(/desired: stopped|desired: running/)).toBeVisible()
  await page.getByRole('link', { name: 'Back' }).click()
  await expect(page).toHaveURL('/machines')

  page.once('dialog', (dialog) => dialog.accept())
  await page.getByRole('button', { name: 'Delete' }).first().click()
  await expect(page.getByText('No machines yet.')).toBeVisible()
})

test('authenticated login route honors next parameter', async ({ page }) => {
  await login(page)

  await page.goto('/login?next=%2Fmachines')
  await expect(page).toHaveURL('/machines')
  await expect(page.getByRole('heading', { name: 'Machines' })).toBeVisible()
})

test('authenticated login route honors nested authorize next parameter', async ({ page }) => {
  await login(page)

  await page.goto(
    '/login?next=%2Fconsole%2Fauthorize%3Ftarget%3Dhttps%253A%252F%252Farca-test3.ryotarai.info%252Fcallback%253Fnext%253D%25252F',
  )
  await expect(page).toHaveURL(
    '/console/authorize?target=https%3A%2F%2Farca-test3.ryotarai.info%2Fcallback%3Fnext%3D%252F',
  )
})
