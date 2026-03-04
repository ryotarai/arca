import { expect, test, type Page } from '@playwright/test'

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

test('redirect path exposes login screen', async ({ page }) => {
  await ensureSetupCompleted(page)
  await page.goto('/')
  await page.getByRole('link', { name: 'Login' }).click()

  await expect(page).toHaveURL('/login')
  await expect(page.getByRole('heading', { name: 'Arca' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Login' })).toBeVisible()
})

test('login route is directly accessible', async ({ page }) => {
  await ensureSetupCompleted(page)
  await page.goto('/login')

  await expect(page).toHaveURL('/login')
  await expect(page.getByLabel('Email')).toBeVisible()
  await expect(page.getByLabel('Password')).toBeVisible()
})

test('unauthenticated user cannot access authenticated dashboard view', async ({ page }) => {
  await ensureSetupCompleted(page)
  await page.goto('/')

  await expect(page.getByRole('link', { name: 'Login' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Dashboard' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Logout' })).toHaveCount(0)
})

test('register, login, and logout via Connect RPC', async ({ page }) => {
  await ensureSetupCompleted(page)
  const email = `e2e-${Date.now()}@example.com`
  const password = 'password123'

  await page.goto('/login')
  await page.getByRole('button', { name: 'Create new account' }).click()
  await page.getByLabel('Email').fill(email)
  await page.getByLabel('Password').fill(password)

  const registerRequest = page.waitForRequest(
    (request) =>
      request.url().endsWith('/arca.v1.AuthService/Register') &&
      request.method() === 'POST',
  )
  await page.getByRole('button', { name: 'Register' }).click()
  await registerRequest

  await expect(page.getByText('registered. please log in.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Login' })).toBeVisible()

  const loginRequest = page.waitForRequest(
    (request) =>
      request.url().endsWith('/arca.v1.AuthService/Login') &&
      request.method() === 'POST',
  )
  await page.getByLabel('Password').fill(password)
  await page.getByRole('button', { name: 'Login' }).click()
  await loginRequest

  await expect(page).toHaveURL('/')
  await expect(page.getByText(`Signed in as ${email}`)).toBeVisible()

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
  await ensureSetupCompleted(page)
  const email = `machine-${Date.now()}@example.com`
  const password = 'password123'
  const machineName = `alpha-machine-${Date.now()}`

  await page.goto('/login')
  await page.getByRole('button', { name: 'Create new account' }).click()
  await page.getByLabel('Email').fill(email)
  await page.getByLabel('Password').fill(password)
  const registerRequest = page.waitForRequest(
    (request) =>
      request.url().endsWith('/arca.v1.AuthService/Register') &&
      request.method() === 'POST',
  )
  await page.getByRole('button', { name: 'Register' }).click()
  await registerRequest
  await expect(page.getByText('registered. please log in.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Login' })).toBeVisible()

  const loginRequest = page.waitForRequest(
    (request) =>
      request.url().endsWith('/arca.v1.AuthService/Login') &&
      request.method() === 'POST',
  )
  await page.getByLabel('Password').fill(password)
  await page.getByRole('button', { name: 'Login' }).click()
  await loginRequest
  await expect(page).toHaveURL('/')

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
  await ensureSetupCompleted(page)
  const email = `next-${Date.now()}@example.com`
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

  await page.goto('/login?next=%2Fmachines')
  await expect(page).toHaveURL('/machines')
  await expect(page.getByRole('heading', { name: 'Machines' })).toBeVisible()
})

test('authenticated login route honors nested authorize next parameter', async ({ page }) => {
  await ensureSetupCompleted(page)
  const email = `nested-next-${Date.now()}@example.com`
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

  await page.goto(
    '/login?next=%2Fconsole%2Fauthorize%3Ftarget%3Dhttps%253A%252F%252Farca-test3.ryotarai.info%252Fcallback%253Fnext%253D%25252F',
  )
  await expect(page).toHaveURL(
    '/console/authorize?target=https%3A%2F%2Farca-test3.ryotarai.info%2Fcallback%3Fnext%3D%252F',
  )
})
