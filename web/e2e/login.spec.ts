import { expect, test } from '@playwright/test'

test('redirect path exposes login screen', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('link', { name: 'Login' }).click()

  await expect(page).toHaveURL('/login')
  await expect(page.getByRole('heading', { name: 'Hayai' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Login' })).toBeVisible()
})

test('login route is directly accessible', async ({ page }) => {
  await page.goto('/login')

  await expect(page).toHaveURL('/login')
  await expect(page.getByLabel('Email')).toBeVisible()
  await expect(page.getByLabel('Password')).toBeVisible()
})

test('unauthenticated user cannot access authenticated dashboard view', async ({ page }) => {
  await page.goto('/')

  await expect(page.getByRole('link', { name: 'Login' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Dashboard' })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Logout' })).toHaveCount(0)
})

test('register, login, and logout via Connect RPC', async ({ page }) => {
  const email = `e2e-${Date.now()}@example.com`
  const password = 'password123'

  await page.goto('/login')
  await page.getByRole('button', { name: 'Create new account' }).click()
  await page.getByLabel('Email').fill(email)
  await page.getByLabel('Password').fill(password)

  const registerRequest = page.waitForRequest(
    (request) =>
      request.url().endsWith('/hayai.v1.AuthService/Register') &&
      request.method() === 'POST',
  )
  await page.getByRole('button', { name: 'Register' }).click()
  await registerRequest

  await expect(page.getByText('registered. please log in.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Login' })).toBeVisible()

  const loginRequest = page.waitForRequest(
    (request) =>
      request.url().endsWith('/hayai.v1.AuthService/Login') &&
      request.method() === 'POST',
  )
  await page.getByLabel('Password').fill(password)
  await page.getByRole('button', { name: 'Login' }).click()
  await loginRequest

  await expect(page).toHaveURL('/')
  await expect(page.getByText(`Signed in as ${email}`)).toBeVisible()

  const logoutRequest = page.waitForRequest(
    (request) =>
      request.url().endsWith('/hayai.v1.AuthService/Logout') &&
      request.method() === 'POST',
  )
  await page.getByRole('button', { name: 'Logout' }).click()
  await logoutRequest

  await expect(page.getByRole('link', { name: 'Login' })).toBeVisible()
})

test('machine CRUD screen works for authenticated user', async ({ page }) => {
  const email = `machine-${Date.now()}@example.com`
  const password = 'password123'

  await page.goto('/login')
  await page.getByRole('button', { name: 'Create new account' }).click()
  await page.getByLabel('Email').fill(email)
  await page.getByLabel('Password').fill(password)
  const registerRequest = page.waitForRequest(
    (request) =>
      request.url().endsWith('/hayai.v1.AuthService/Register') &&
      request.method() === 'POST',
  )
  await page.getByRole('button', { name: 'Register' }).click()
  await registerRequest
  await expect(page.getByText('registered. please log in.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Login' })).toBeVisible()

  const loginRequest = page.waitForRequest(
    (request) =>
      request.url().endsWith('/hayai.v1.AuthService/Login') &&
      request.method() === 'POST',
  )
  await page.getByLabel('Password').fill(password)
  await page.getByRole('button', { name: 'Login' }).click()
  await loginRequest
  await expect(page).toHaveURL('/')

  await page.getByRole('link', { name: 'Machines' }).click()
  await expect(page).toHaveURL('/machines')
  await expect(page.getByRole('heading', { name: 'Machines' })).toBeVisible()

  await page.getByLabel('Name').fill('alpha-machine')
  await page.getByRole('button', { name: 'Create' }).click()
  await expect(page.getByText('alpha-machine')).toBeVisible()
  await expect(page.getByText(/status: (pending|starting|running)/)).toBeVisible()

  await page.getByRole('button', { name: 'Edit' }).first().click()
  await page.getByLabel('Edit machine name').fill('beta-machine')
  await page.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByText('beta-machine')).toBeVisible()

  await page.getByRole('button', { name: 'Stop' }).first().click()
  await expect(page.getByText(/status: stopping/)).toBeVisible()

  await page.getByRole('button', { name: 'Delete' }).first().click()
  await expect(page.getByText('No machines yet.')).toBeVisible()
})
