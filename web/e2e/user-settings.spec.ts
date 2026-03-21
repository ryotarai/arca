import { expect, test } from '@playwright/test'
import { loginAsAdmin } from './helpers/auth'

test.describe('user settings', () => {
  test('settings page shows user settings heading', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/settings')
    await expect(page.getByRole('heading', { name: 'User settings' })).toBeVisible()
  })

  test('startup script card is visible and saves script', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/settings')
    await expect(page.getByText('Startup script', { exact: true })).toBeVisible()

    const textarea = page.getByPlaceholder('#!/bin/bash')
    await textarea.fill('echo hello')
    await page.getByRole('button', { name: 'Save startup script' }).click()
    await expect(page.getByText('Startup script updated.')).toBeVisible()

    // Reload and verify persistence
    await page.reload()
    await expect(textarea).toHaveValue('echo hello')
  })
})
