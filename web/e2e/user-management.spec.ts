import { expect, test } from '@playwright/test'
import {
  completeUserSetup,
  createAuthedUserContext,
  createSecondaryUser,
  loginAsAdmin,
} from './helpers/auth'

test.describe('user management', () => {
  test('admin users page shows heading and form', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/users')
    await expect(page).toHaveURL('/users')
    await expect(page.getByRole('heading', { name: 'Users' })).toBeVisible()
    await expect(page.locator('#user-email')).toBeVisible()
    await expect(page.getByRole('button', { name: 'Create user' })).toBeVisible()
  })

  test('creating a user shows setup token', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/users')

    const email = `newuser-${Date.now()}@example.com`
    await page.locator('#user-email').fill(email)
    await page.getByRole('button', { name: 'Create user' }).click()

    await expect(page.getByText('One-time setup token', { exact: true })).toBeVisible()
    await expect(page.getByText(email, { exact: true })).toBeVisible()
  })

  test('reissue setup token', async ({ page }) => {
    await loginAsAdmin(page)
    const email = `reissue-${Date.now()}@example.com`
    await createSecondaryUser(page, email)

    await page.goto('/users')
    const userRow = page.locator('.rounded-lg.border', { hasText: email })
    await expect(userRow).toBeVisible()

    await userRow.getByRole('button', { name: 'Issue setup token' }).click()
    await expect(page.getByText('One-time setup token', { exact: true })).toBeVisible()
  })

  test('toggle admin role', async ({ page }) => {
    await loginAsAdmin(page)
    const email = `role-${Date.now()}@example.com`
    await createSecondaryUser(page, email)

    await page.goto('/users')
    const userRow = page.locator('.rounded-lg.border', { hasText: email })
    await expect(userRow).toBeVisible()

    // Make admin
    await userRow.getByRole('button', { name: 'Make admin' }).click()
    await expect(userRow.getByRole('button', { name: 'Revoke admin' })).toBeVisible()

    // Revoke admin
    await userRow.getByRole('button', { name: 'Revoke admin' }).click()
    await expect(userRow.getByRole('button', { name: 'Make admin' })).toBeVisible()
  })

  test('non-admin user cannot access users page', async ({ page, browser }) => {
    await loginAsAdmin(page)
    const email = `nonadmin-usr-${Date.now()}@example.com`
    const { setupToken } = await createSecondaryUser(page, email)
    const userPassword = 'password456'
    await completeUserSetup(page, setupToken, userPassword)

    const ctx = await createAuthedUserContext(
      browser,
      process.env.PLAYWRIGHT_BASE_URL ?? 'http://127.0.0.1:18080',
      email,
      userPassword,
    )
    try {
      await ctx.page.goto('/users')
      // Non-admin is redirected to /machines
      await expect(ctx.page).toHaveURL('/machines')
    } finally {
      await ctx.close()
    }
  })

  test('user setup: valid token sets password and allows login', async ({ page, browser }) => {
    await loginAsAdmin(page)
    const email = `setup-${Date.now()}@example.com`
    const { setupToken } = await createSecondaryUser(page, email)
    const userPassword = 'setup-password-123'

    // Complete setup via UI
    const setupCtx = await browser.newContext({
      baseURL: process.env.PLAYWRIGHT_BASE_URL ?? 'http://127.0.0.1:18080',
    })
    const setupPage = await setupCtx.newPage()
    try {
      await setupPage.goto(`/users/setup?token=${setupToken}`)
      await expect(setupPage.getByText('Complete account setup')).toBeVisible()
      await setupPage.locator('#setup-password').fill(userPassword)
      await setupPage.locator('#setup-password-confirm').fill(userPassword)
      await setupPage.getByRole('button', { name: 'Set password' }).click()
      await expect(setupPage.getByText('Password is set')).toBeVisible()
    } finally {
      await setupCtx.close()
    }

    // Verify the user can log in
    const ctx = await createAuthedUserContext(
      browser,
      process.env.PLAYWRIGHT_BASE_URL ?? 'http://127.0.0.1:18080',
      email,
      userPassword,
    )
    try {
      await expect(ctx.page).toHaveURL('/machines')
    } finally {
      await ctx.close()
    }
  })
})
