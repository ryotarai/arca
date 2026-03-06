import { expect, test } from '@playwright/test'
import { adminEmail, adminPassword, ensureSetupAdmin, loginAsAdmin } from './helpers'

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
  await loginAsAdmin(page)

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
  await loginAsAdmin(page)

  await page.goto('/login?next=%2Fmachines')
  await expect(page).toHaveURL('/machines')
  await expect(page.getByRole('heading', { name: 'Machines' })).toBeVisible()
})

test('authenticated login route honors nested authorize next parameter', async ({ page }) => {
  await loginAsAdmin(page)

  await page.goto(
    '/login?next=%2Fconsole%2Fauthorize%3Ftarget%3Dhttps%253A%252F%252Farca-test3.ryotarai.info%252Fcallback%253Fnext%253D%25252F',
  )
  await expect(page).toHaveURL(
    '/console/authorize?target=https%3A%2F%2Farca-test3.ryotarai.info%2Fcallback%3Fnext%3D%252F',
  )
})
