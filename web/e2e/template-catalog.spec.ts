import { expect, test } from '@playwright/test'
import {
  completeUserSetup,
  createAuthedUserContext,
  createSecondaryUser,
  loginAsAdmin,
} from './helpers/auth'

test.describe('template catalog', () => {
  test('templates list page shows heading and new button', async ({ page }) => {
    await loginAsAdmin(page)
    await page.getByRole('link', { name: 'Machine Templates' }).click()
    await expect(page).toHaveURL('/machine-templates')
    await expect(page.getByRole('heading', { name: 'Machine Templates' })).toBeVisible()
    await expect(page.getByRole('link', { name: 'New template' })).toBeVisible()
  })

  test('new template form page shows name and type fields', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/machine-templates/new')
    await expect(page.getByRole('heading', { name: 'New template' })).toBeVisible()
    await expect(page.locator('#template-name')).toBeVisible()
    await expect(page.locator('#template-type')).toBeVisible()
  })

  test('create LXD template with required fields', async ({ page }) => {
    const templateName = `lxd-rt-${Date.now()}`
    await loginAsAdmin(page)
    await page.goto('/machine-templates/new')

    await page.locator('#template-name').fill(templateName)
    await page.locator('#template-type').selectOption('lxd')
    await page.locator('#template-lxd-endpoint').fill('https://localhost:8443')

    const submitButton = page.getByRole('button', { name: 'Create template' })
    await expect(submitButton).toBeEnabled()
    await submitButton.click()

    // Should redirect to detail page
    await expect(page.getByRole('heading', { name: 'Template detail' })).toBeVisible()
    await expect(page.getByText(templateName)).toBeVisible()

    // cleanup via detail page delete
    page.once('dialog', (dialog) => dialog.accept())
    await page.getByRole('link', { name: 'Delete' }).or(page.getByRole('button', { name: 'Delete' })).click()
    await expect(page).toHaveURL('/machine-templates')
  })

  test('create libvirt template with required fields', async ({ page }) => {
    const templateName = `libvirt-rt-${Date.now()}`
    await loginAsAdmin(page)
    await page.goto('/machine-templates/new')

    await page.locator('#template-name').fill(templateName)
    // libvirt is the default type, just fill required fields
    await page.getByLabel('URI').fill('qemu:///system')
    await page.getByLabel('Network').first().fill('default')
    await page.getByLabel('Storage pool').fill('default')

    const submitButton = page.getByRole('button', { name: 'Create template' })
    await expect(submitButton).toBeEnabled()
    await submitButton.click()

    // Should redirect to detail page
    await expect(page.getByRole('heading', { name: 'Template detail' })).toBeVisible()
    await expect(page.getByText(templateName)).toBeVisible()

    // cleanup via detail page delete
    page.once('dialog', (dialog) => dialog.accept())
    await page.getByRole('button', { name: 'Delete' }).click()
    await expect(page).toHaveURL('/machine-templates')
  })

  test('type change (libvirt to GCE) validates type-specific fields', async ({ page }) => {
    const templateName = `type-change-${Date.now()}`
    await loginAsAdmin(page)
    await page.goto('/machine-templates/new')

    await page.locator('#template-name').fill(templateName)
    await page.getByLabel('URI').fill('qemu:///system')
    await page.getByLabel('Network').first().fill('default')
    await page.getByLabel('Storage pool').fill('default')

    await page.getByRole('button', { name: 'Create template' }).click()

    // Redirected to detail page; navigate to edit
    await expect(page.getByRole('heading', { name: 'Template detail' })).toBeVisible()
    await page.getByRole('link', { name: 'Edit' }).click()

    await page.getByLabel('Type').selectOption('gce')
    await expect(page.getByRole('button', { name: 'Save template' })).toBeDisabled()

    await page.getByLabel('Project', { exact: true }).fill('my-project')
    await page.getByLabel('Zone', { exact: true }).fill('us-central1-a')
    await page.getByLabel('Network').first().fill('vpc-main')
    await page.getByLabel('Subnetwork').fill('subnet-main')
    await page.getByLabel('Service account email').fill('svc@example.iam.gserviceaccount.com')
    await page.getByLabel('Machine types').fill('e2-standard-2')
    await page.getByRole('button', { name: 'Save template' }).click()

    // Redirected back to detail; verify type changed
    await expect(page.getByRole('heading', { name: 'Template detail' })).toBeVisible()
    await expect(page.getByText('gce')).toBeVisible()

    // cleanup
    page.once('dialog', (dialog) => dialog.accept())
    await page.getByRole('button', { name: 'Delete' }).click()
    await expect(page).toHaveURL('/machine-templates')
  })

  test('template deletion removes from catalog', async ({ page }) => {
    const templateName = `del-rt-${Date.now()}`
    await loginAsAdmin(page)
    await page.goto('/machine-templates/new')

    await page.locator('#template-name').fill(templateName)
    await page.locator('#template-type').selectOption('lxd')
    await page.locator('#template-lxd-endpoint').fill('https://localhost:8443')
    await page.getByRole('button', { name: 'Create template' }).click()

    // Redirected to detail page; delete from there
    await expect(page.getByRole('heading', { name: 'Template detail' })).toBeVisible()
    page.once('dialog', (dialog) => dialog.accept())
    await page.getByRole('button', { name: 'Delete' }).click()
    await expect(page).toHaveURL('/machine-templates')
    await expect(page.getByText(templateName)).toHaveCount(0)
  })

  test('form validation prevents short name submission', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/machine-templates/new')

    const submitButton = page.getByRole('button', { name: 'Create template' })
    await expect(submitButton).toBeDisabled()

    await page.locator('#template-name').fill('ab')
    await expect(submitButton).toBeDisabled()
  })

  test('Proxy via server exposure shows Connectivity', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/machine-templates/new')

    await expect(page.locator('#template-exposure-connectivity')).toBeVisible()
  })

  test('create template with proxy via server exposure', async ({ page }) => {
    const templateName = `proxy-rt-${Date.now()}`
    await loginAsAdmin(page)
    await page.goto('/machine-templates/new')

    await page.locator('#template-name').fill(templateName)
    await page.locator('#template-type').selectOption('lxd')
    await page.locator('#template-lxd-endpoint').fill('https://localhost:8443')
    await page.locator('#template-exposure-domain-prefix').fill('arca-')
    await page.locator('#template-exposure-base-domain').fill('localhost')
    await page.locator('#template-exposure-connectivity').selectOption('private_ip')

    await page.getByRole('button', { name: 'Create template' }).click()

    // Should redirect to detail page
    await expect(page.getByRole('heading', { name: 'Template detail' })).toBeVisible()
    await expect(page.getByText(templateName)).toBeVisible()

    // cleanup
    page.once('dialog', (dialog) => dialog.accept())
    await page.getByRole('button', { name: 'Delete' }).click()
    await expect(page).toHaveURL('/machine-templates')
  })

  test('non-admin user cannot access templates page', async ({ page, browser }) => {
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
      await ctx.page.goto('/machine-templates')
      // Non-admin should be redirected away or see no admin content
      await expect(ctx.page.getByRole('heading', { name: 'Machine Templates' })).toHaveCount(0)
    } finally {
      await ctx.close()
    }
  })
})
