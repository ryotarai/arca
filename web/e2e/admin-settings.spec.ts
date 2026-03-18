import { expect, test } from '@playwright/test'
import {
  completeUserSetup,
  createAuthedUserContext,
  createSecondaryUser,
  loginAsAdmin,
} from './helpers/auth'

test.describe('admin settings', () => {
  test('admin settings page shows all sections', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/admin/settings')
    await expect(page.getByRole('heading', { name: 'Admin settings' })).toBeVisible()
    await expect(page.getByText('Domain settings')).toBeVisible()
    await expect(page.locator('#settings-password-login-enabled')).toBeVisible()
    await expect(page.locator('#settings-oidc-enabled')).toBeVisible()
    await expect(page.locator('#settings-iap-enabled')).toBeVisible()
  })

  test('save and roundtrip settings', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/admin/settings')
    await expect(page.getByRole('heading', { name: 'Admin settings' })).toBeVisible()

    // Toggle the disable-internet-public checkbox (simpler toggle, no side effects)
    const checkbox = page.locator('#settings-disable-internet-public')
    await expect(checkbox).toBeVisible()
    const initialChecked = await checkbox.isChecked()

    if (initialChecked) {
      await checkbox.uncheck()
    } else {
      await checkbox.check()
    }

    await page.getByRole('button', { name: 'Save settings' }).click()
    await expect(page.getByText('Settings updated')).toBeVisible()

    // Reload and verify roundtrip
    await page.goto('/admin/settings')
    if (initialChecked) {
      await expect(checkbox).not.toBeChecked()
    } else {
      await expect(checkbox).toBeChecked()
    }

    // Restore original state
    if (initialChecked) {
      await checkbox.check()
    } else {
      await checkbox.uncheck()
    }
    await page.getByRole('button', { name: 'Save settings' }).click()
    await expect(page.getByText('Settings updated')).toBeVisible()
  })

  test('non-admin user cannot access admin settings', async ({ page, browser }) => {
    await loginAsAdmin(page)
    const email = `nonadmin-settings-${Date.now()}@example.com`
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
      await ctx.page.goto('/admin/settings')
      // Non-admin is redirected to /settings (user settings)
      await expect(ctx.page).toHaveURL('/settings')
    } finally {
      await ctx.close()
    }
  })
})
