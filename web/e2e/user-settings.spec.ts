import { expect, test } from '@playwright/test'
import { loginAsAdmin } from './helpers/auth'

// Valid SSH key for testing
const validSSHKey =
  'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN8zCLGmiJswZcMk2SD3Aye+Zabnf1CT4TrJZsK937Zd test@example.com'

test.describe('user settings', () => {
  test('settings page shows SSH public keys form', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/settings')
    await expect(page.getByRole('heading', { name: 'User settings' })).toBeVisible()
    await expect(page.getByText('SSH public keys', { exact: true })).toBeVisible()
    await expect(page.locator('#settings-ssh-public-keys')).toBeVisible()
    await expect(page.getByRole('button', { name: 'Save SSH keys' })).toBeVisible()
  })

  test('save and persist SSH public keys', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/settings')

    await page.locator('#settings-ssh-public-keys').fill(validSSHKey)
    await page.getByRole('button', { name: 'Save SSH keys' }).click()
    await expect(page.getByText('SSH keys updated')).toBeVisible()

    // Reload and verify persistence
    await page.goto('/settings')
    await expect(page.locator('#settings-ssh-public-keys')).toHaveValue(validSSHKey)
  })

  test('clear SSH public keys', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/settings')

    // First set a key
    await page.locator('#settings-ssh-public-keys').fill(validSSHKey)
    await page.getByRole('button', { name: 'Save SSH keys' }).click()
    await expect(page.getByText('SSH keys updated')).toBeVisible()

    // Clear it
    await page.locator('#settings-ssh-public-keys').fill('')
    await page.getByRole('button', { name: 'Save SSH keys' }).click()
    await expect(page.getByText('SSH keys updated')).toBeVisible()

    // Verify cleared
    await page.goto('/settings')
    await expect(page.locator('#settings-ssh-public-keys')).toHaveValue('')
  })
})
