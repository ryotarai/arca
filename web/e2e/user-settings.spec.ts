import { expect, test } from '@playwright/test'
import { loginAsAdmin } from './helpers/auth'

test.describe('user settings', () => {
  test('settings page shows user settings heading', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/settings')
    await expect(page.getByRole('heading', { name: 'User settings' })).toBeVisible()
  })
})
