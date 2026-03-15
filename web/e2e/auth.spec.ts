import { expect, test } from '@playwright/test'
import { adminEmail, adminPassword, ensureSetupAdmin, loginAsAdmin } from './helpers/auth'

test('redirect path exposes login screen', async ({ page }) => {
  await ensureSetupAdmin(page)
  await page.goto('/')

  await expect(page).toHaveURL('/login?next=%2F')
  await expect(page.getByRole('heading', { name: 'Arca' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Login' })).toBeVisible()
})

test('login route is directly accessible', async ({ page }) => {
  await ensureSetupAdmin(page)
  await page.goto('/login')

  await expect(page).toHaveURL('/login')
  await expect(page.getByLabel('Email')).toBeVisible()
  await expect(page.getByLabel('Password', { exact: true })).toBeVisible()
})

test('unauthenticated user cannot access authenticated dashboard view', async ({ page }) => {
  await ensureSetupAdmin(page)
  await page.goto('/')

  await expect(page).toHaveURL('/login?next=%2F')
  await expect(page.getByRole('button', { name: 'Login' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Logout' })).toHaveCount(0)
})

test('login and logout via Connect RPC', async ({ page }) => {
  await ensureSetupAdmin(page)

  await page.goto('/login')
  await page.getByLabel('Email').fill(adminEmail)
  await page.getByLabel('Password', { exact: true }).fill(adminPassword)

  const loginRequest = page.waitForRequest(
    (request) =>
      request.url().endsWith('/arca.v1.AuthService/Login') && request.method() === 'POST',
  )
  await page.getByRole('button', { name: 'Login' }).click()
  await loginRequest

  await expect(page).toHaveURL('/machines')
  await expect(page.getByText(`Signed in as ${adminEmail}`)).toBeVisible()

  const logoutRequest = page.waitForRequest(
    (request) =>
      request.url().endsWith('/arca.v1.AuthService/Logout') && request.method() === 'POST',
  )
  await page.getByRole('button', { name: 'Logout' }).first().click()
  await logoutRequest

  await expect(page).toHaveURL('/login?next=%2Fmachines')
  await expect(page.getByRole('button', { name: 'Login' })).toBeVisible()
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

test('wrong password shows error', async ({ page }) => {
  await ensureSetupAdmin(page)

  await page.goto('/login')
  await page.getByLabel('Email').fill(adminEmail)
  await page.getByLabel('Password', { exact: true }).fill('wrong-password')
  await page.getByRole('button', { name: 'Login' }).click()

  await expect(page.getByRole('alert')).toBeVisible()
})
