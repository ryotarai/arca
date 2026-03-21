import { expect, test } from '@playwright/test'
import {
  completeUserSetup,
  createAuthedUserContext,
  createSecondaryUser,
  loginAsAdmin,
} from './helpers/auth'

test.describe('profile catalog', () => {
  test('profiles list page shows heading and new button', async ({ page }) => {
    await loginAsAdmin(page)
    await page.getByRole('link', { name: 'Machine Profiles' }).click()
    await expect(page).toHaveURL('/machine-profiles')
    await expect(page.getByRole('heading', { name: 'Machine Profiles' })).toBeVisible()
    await expect(page.getByRole('link', { name: 'New profile' })).toBeVisible()
  })

  test('new profile form page shows name and type fields', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/machine-profiles/new')
    await expect(page.getByRole('heading', { name: 'New profile' })).toBeVisible()
    await expect(page.locator('#profile-name')).toBeVisible()
    await expect(page.locator('#profile-type')).toBeVisible()
  })

  test('create LXD profile with required fields', async ({ page }) => {
    const profileName = `lxd-rt-${Date.now()}`
    await loginAsAdmin(page)
    await page.goto('/machine-profiles/new')

    await page.locator('#profile-name').fill(profileName)
    await page.locator('#profile-type').selectOption('lxd')
    await page.locator('#profile-lxd-endpoint').fill('https://localhost:8443')

    const submitButton = page.getByRole('button', { name: 'Create profile' })
    await expect(submitButton).toBeEnabled()
    await submitButton.click()

    // Should redirect to detail page
    await expect(page.getByRole('heading', { name: 'Profile detail' })).toBeVisible()
    await expect(page.getByText(profileName)).toBeVisible()

    // cleanup via detail page delete
    await page.getByRole('link', { name: 'Delete' }).or(page.getByRole('button', { name: 'Delete' })).click()
    await page.getByRole('alertdialog').getByRole('button', { name: 'Delete' }).click()
    await expect(page).toHaveURL('/machine-profiles')
  })

  test('create libvirt profile with required fields', async ({ page }) => {
    const profileName = `libvirt-rt-${Date.now()}`
    await loginAsAdmin(page)
    await page.goto('/machine-profiles/new')

    await page.locator('#profile-name').fill(profileName)
    // libvirt is the default type, just fill required fields
    await page.getByLabel('URI').fill('qemu:///system')
    await page.getByLabel('Network').first().fill('default')
    await page.getByLabel('Storage pool').fill('default')

    const submitButton = page.getByRole('button', { name: 'Create profile' })
    await expect(submitButton).toBeEnabled()
    await submitButton.click()

    // Should redirect to detail page
    await expect(page.getByRole('heading', { name: 'Profile detail' })).toBeVisible()
    await expect(page.getByText(profileName)).toBeVisible()

    // cleanup via detail page delete
    await page.getByRole('button', { name: 'Delete' }).click()
    await page.getByRole('alertdialog').getByRole('button', { name: 'Delete' }).click()
    await expect(page).toHaveURL('/machine-profiles')
  })

  test('type change (libvirt to GCE) validates type-specific fields', async ({ page }) => {
    const profileName = `type-change-${Date.now()}`
    await loginAsAdmin(page)
    await page.goto('/machine-profiles/new')

    await page.locator('#profile-name').fill(profileName)
    await page.getByLabel('URI').fill('qemu:///system')
    await page.getByLabel('Network').first().fill('default')
    await page.getByLabel('Storage pool').fill('default')

    await page.getByRole('button', { name: 'Create profile' }).click()

    // Redirected to detail page; navigate to edit
    await expect(page.getByRole('heading', { name: 'Profile detail' })).toBeVisible()
    await page.getByRole('link', { name: 'Edit' }).click()

    await page.getByLabel('Type').selectOption('gce')
    await expect(page.getByRole('button', { name: 'Save profile' })).toBeDisabled()

    await page.getByLabel('Project', { exact: true }).fill('my-project')
    await page.getByLabel('Zone', { exact: true }).fill('us-central1-a')
    await page.getByLabel('Network').first().fill('vpc-main')
    await page.getByLabel('Subnetwork').fill('subnet-main')
    await page.getByLabel('Service account email').fill('svc@example.iam.gserviceaccount.com')
    await page.getByLabel('Machine types').fill('e2-standard-2')
    await page.getByRole('button', { name: 'Save profile' }).click()

    // Redirected back to detail; verify type changed
    await expect(page.getByRole('heading', { name: 'Profile detail' })).toBeVisible()
    await expect(page.getByText('gce')).toBeVisible()

    // cleanup
    await page.getByRole('button', { name: 'Delete' }).click()
    await page.getByRole('alertdialog').getByRole('button', { name: 'Delete' }).click()
    await expect(page).toHaveURL('/machine-profiles')
  })

  test('profile deletion removes from catalog', async ({ page }) => {
    const profileName = `del-rt-${Date.now()}`
    await loginAsAdmin(page)
    await page.goto('/machine-profiles/new')

    await page.locator('#profile-name').fill(profileName)
    await page.locator('#profile-type').selectOption('lxd')
    await page.locator('#profile-lxd-endpoint').fill('https://localhost:8443')
    await page.getByRole('button', { name: 'Create profile' }).click()

    // Redirected to detail page; delete from there
    await expect(page.getByRole('heading', { name: 'Profile detail' })).toBeVisible()
    await page.getByRole('button', { name: 'Delete' }).click()
    await page.getByRole('alertdialog').getByRole('button', { name: 'Delete' }).click()
    await expect(page).toHaveURL('/machine-profiles')
    await expect(page.getByText(profileName)).toHaveCount(0)
  })

  test('form validation prevents short name submission', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/machine-profiles/new')

    const submitButton = page.getByRole('button', { name: 'Create profile' })
    await expect(submitButton).toBeDisabled()

    await page.locator('#profile-name').fill('ab')
    await expect(submitButton).toBeDisabled()
  })

  test('Proxy via server exposure shows Connectivity', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/machine-profiles/new')

    await expect(page.locator('#profile-exposure-connectivity')).toBeVisible()
  })

  test('create profile with proxy via server exposure', async ({ page }) => {
    const profileName = `proxy-rt-${Date.now()}`
    await loginAsAdmin(page)
    await page.goto('/machine-profiles/new')

    await page.locator('#profile-name').fill(profileName)
    await page.locator('#profile-type').selectOption('lxd')
    await page.locator('#profile-lxd-endpoint').fill('https://localhost:8443')
    await page.locator('#profile-exposure-connectivity').selectOption('private_ip')

    await page.getByRole('button', { name: 'Create profile' }).click()

    // Should redirect to detail page
    await expect(page.getByRole('heading', { name: 'Profile detail' })).toBeVisible()
    await expect(page.getByText(profileName)).toBeVisible()

    // cleanup
    await page.getByRole('button', { name: 'Delete' }).click()
    await page.getByRole('alertdialog').getByRole('button', { name: 'Delete' }).click()
    await expect(page).toHaveURL('/machine-profiles')
  })

  test('non-admin user cannot access profiles page', async ({ page, browser }) => {
    await loginAsAdmin(page)
    const { setupToken } = await createSecondaryUser(page, `nonadmin-rt-${Date.now()}@example.com`)
    const userPassword = 'password456'
    await completeUserSetup(page, setupToken, userPassword)

    const userEmail = `nonadmin-rt-${Date.now()}@example.com`
    // We already created a user above, use a fresh one for login
    const { setupToken: st2 } = await createSecondaryUser(page, userEmail)
    await completeUserSetup(page, st2, userPassword)

    const ctx = await createAuthedUserContext(
      browser,
      process.env.PLAYWRIGHT_BASE_URL ?? 'http://127.0.0.1:18080',
      userEmail,
      userPassword,
    )
    try {
      await ctx.page.goto('/machine-profiles')
      // Non-admin should be redirected away or see no admin content
      await expect(ctx.page.getByRole('heading', { name: 'Machine Profiles' })).toHaveCount(0)
    } finally {
      await ctx.close()
    }
  })
})
