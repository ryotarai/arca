import { expect, test } from '@playwright/test'
import {
  completeUserSetup,
  createAuthedUserContext,
  createSecondaryUser,
  loginAsAdmin,
} from './helpers/auth'

test.describe('navigation', () => {
  test('admin sidebar shows Machine Templates, Users, and Admin settings links', async ({ page }) => {
    await loginAsAdmin(page)

    await expect(page.getByRole('link', { name: 'Machines' })).toBeVisible()
    await expect(page.getByRole('link', { name: 'User settings' })).toBeVisible()
    await expect(page.getByRole('link', { name: 'Machine Templates' })).toBeVisible()
    await expect(page.getByRole('link', { name: 'Users' })).toBeVisible()
    await expect(page.getByRole('link', { name: 'Groups' })).toBeVisible()
    await expect(page.getByRole('link', { name: 'Admin settings' })).toBeVisible()
  })

  test('non-admin sidebar hides admin menu items', async ({ page, browser }) => {
    await loginAsAdmin(page)
    const email = `nav-user-${Date.now()}@example.com`
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
      await expect(ctx.page.getByRole('link', { name: 'Machines' })).toBeVisible()
      await expect(ctx.page.getByRole('link', { name: 'User settings' })).toBeVisible()
      // Admin links should not be visible
      await expect(ctx.page.getByRole('link', { name: 'Machine Templates' })).toHaveCount(0)
      await expect(ctx.page.getByRole('link', { name: 'Users' })).toHaveCount(0)
      await expect(ctx.page.getByRole('link', { name: 'Groups' })).toHaveCount(0)
      await expect(ctx.page.getByRole('link', { name: 'Admin settings' })).toHaveCount(0)
    } finally {
      await ctx.close()
    }
  })

  test('sidebar Logout button works', async ({ page }) => {
    await loginAsAdmin(page)

    await page.getByRole('button', { name: 'Logout' }).first().click()
    await expect(page).toHaveURL(/\/login/)
    await expect(page.getByRole('button', { name: 'Login' })).toBeVisible()
  })

  test('nav links navigate to correct pages', async ({ page }) => {
    await loginAsAdmin(page)

    await page.getByRole('link', { name: 'Machines' }).click()
    await expect(page).toHaveURL('/machines')
    await expect(page.getByRole('heading', { name: 'Machines' })).toBeVisible()

    await page.getByRole('link', { name: 'Machine Templates' }).click()
    await expect(page).toHaveURL('/machine-templates')
    await expect(page.getByRole('heading', { name: 'Machine Templates' })).toBeVisible()

    await page.getByRole('link', { name: 'Users' }).click()
    await expect(page).toHaveURL('/users')
    await expect(page.getByRole('heading', { name: 'Users' })).toBeVisible()

    await page.getByRole('link', { name: 'Groups' }).click()
    await expect(page).toHaveURL('/groups')
    await expect(page.getByRole('heading', { name: 'Groups' })).toBeVisible()

    await page.getByRole('link', { name: 'Admin settings' }).click()
    await expect(page).toHaveURL('/admin/settings')
    await expect(page.getByRole('heading', { name: 'Admin settings' })).toBeVisible()

    await page.getByRole('link', { name: 'User settings' }).click()
    await expect(page).toHaveURL('/settings')
    await expect(page.getByRole('heading', { name: 'User settings' })).toBeVisible()
  })

  test('non-admin direct URL access to admin pages is blocked', async ({ page, browser }) => {
    await loginAsAdmin(page)
    const email = `nav-guard-${Date.now()}@example.com`
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
      await ctx.page.goto('/machine-templates')
      await expect(ctx.page.getByRole('heading', { name: 'Machine Templates' })).toHaveCount(0)

      await ctx.page.goto('/users')
      // Non-admin is redirected to /machines
      await expect(ctx.page).toHaveURL('/machines')

      await ctx.page.goto('/groups')
      // Non-admin is redirected to /machines
      await expect(ctx.page).toHaveURL('/machines')

      await ctx.page.goto('/admin/settings')
      // Non-admin is redirected to /settings
      await expect(ctx.page).toHaveURL('/settings')
    } finally {
      await ctx.close()
    }
  })
})
