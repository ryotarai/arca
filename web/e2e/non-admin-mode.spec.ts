import { expect, test } from '@playwright/test'
import { loginAsAdmin } from './helpers/auth'

test.describe('non-admin mode', () => {
  test('admin can enter non-admin mode from admin settings page', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/admin/settings')
    await expect(page.getByRole('heading', { name: 'Admin settings' })).toBeVisible()

    // The "View mode" card should be visible
    await expect(page.getByText('View mode')).toBeVisible()
    await expect(page.getByText('Switch to non-admin mode')).toBeVisible()

    // Click "Enter non-admin mode" button
    await page.getByRole('button', { name: 'Enter non-admin mode' }).click()

    // After page reload, the non-admin mode banner should appear
    await expect(page.getByText('You are in non-admin mode')).toBeVisible()
    await expect(page.getByRole('button', { name: 'Back to admin' })).toBeVisible()
  })

  test('admin menu items are hidden in non-admin mode', async ({ page }) => {
    await loginAsAdmin(page)

    // Enter non-admin mode via API
    await page.request.post('/arca.v1.AdminService/SetAdminViewMode', {
      data: { mode: 'user' },
    })

    // Reload to pick up non-admin mode
    await page.goto('/machines')
    await expect(page.getByText('You are in non-admin mode')).toBeVisible()

    // Non-admin navigation items should still be visible
    await expect(page.getByRole('link', { name: 'Machines' })).toBeVisible()
    await expect(page.getByRole('link', { name: 'User settings' })).toBeVisible()

    // Admin navigation items should be hidden
    await expect(page.getByRole('link', { name: 'Machine Profiles' })).toHaveCount(0)
    await expect(page.getByRole('link', { name: 'Users' })).toHaveCount(0)
    await expect(page.getByRole('link', { name: 'Groups' })).toHaveCount(0)
    await expect(page.getByRole('link', { name: 'Admin settings' })).toHaveCount(0)
    await expect(page.getByRole('link', { name: 'Audit logs' })).toHaveCount(0)
  })

  test('direct URL access to admin pages redirects in non-admin mode', async ({ page }) => {
    await loginAsAdmin(page)

    // Enter non-admin mode via API
    await page.request.post('/arca.v1.AdminService/SetAdminViewMode', {
      data: { mode: 'user' },
    })

    // /admin/settings redirects to /settings
    await page.goto('/admin/settings')
    await expect(page).toHaveURL('/settings')

    // /users redirects to /machines
    await page.goto('/users')
    await expect(page).toHaveURL('/machines')

    // /groups redirects to /machines
    await page.goto('/groups')
    await expect(page).toHaveURL('/machines')

    // /machine-profiles redirects to /machines
    await page.goto('/machine-profiles')
    await expect(page).toHaveURL('/machines')
  })

  test('clicking Back to admin restores admin access', async ({ page }) => {
    await loginAsAdmin(page)

    // Enter non-admin mode via API
    await page.request.post('/arca.v1.AdminService/SetAdminViewMode', {
      data: { mode: 'user' },
    })

    // Reload to pick up non-admin mode
    await page.goto('/machines')
    await expect(page.getByText('You are in non-admin mode')).toBeVisible()

    // Click "Back to admin"
    await page.getByRole('button', { name: 'Back to admin' }).click()

    // After page reload, the banner should be gone
    await expect(page.getByText('You are in non-admin mode')).toHaveCount(0)

    // Admin settings should be accessible again
    await page.goto('/admin/settings')
    await expect(page.getByRole('heading', { name: 'Admin settings' })).toBeVisible()
  })

  test('admin API calls are blocked in non-admin mode', async ({ page }) => {
    await loginAsAdmin(page)

    // Enter non-admin mode via API
    await page.request.post('/arca.v1.AdminService/SetAdminViewMode', {
      data: { mode: 'user' },
    })

    // Admin endpoints should return permission denied
    const listAuditLogsResponse = await page.request.post(
      '/arca.v1.AdminService/ListAuditLogs',
      { data: {} },
    )
    expect(listAuditLogsResponse.ok()).toBe(false)
    const body = await listAuditLogsResponse.json()
    expect(body.code).toBe('permission_denied')

    // GetAdminViewMode should still work (returns view mode info)
    const viewModeResponse = await page.request.post(
      '/arca.v1.AdminService/GetAdminViewMode',
      { data: {} },
    )
    expect(viewModeResponse.ok()).toBe(true)
    const viewMode = await viewModeResponse.json()
    expect(viewMode.mode).toBe('user')
    expect(viewMode.isAdmin).toBe(true)

    // SetAdminViewMode should still work (so admin can switch back)
    const switchBackResponse = await page.request.post(
      '/arca.v1.AdminService/SetAdminViewMode',
      { data: { mode: 'admin' } },
    )
    expect(switchBackResponse.ok()).toBe(true)
  })

  test('non-admin mode banner does not appear for regular users', async ({ page }) => {
    await loginAsAdmin(page)

    // Ensure admin is in normal admin mode
    await page.request.post('/arca.v1.AdminService/SetAdminViewMode', {
      data: { mode: 'admin' },
    })

    await page.goto('/machines')
    await expect(page.getByText('You are in non-admin mode')).toHaveCount(0)
  })

  test.afterEach(async ({ page }) => {
    // Clean up: reset admin view mode to admin
    try {
      await page.request.post('/arca.v1.AdminService/SetAdminViewMode', {
        data: { mode: 'admin' },
      })
    } catch {
      // ignore cleanup errors
    }
  })
})
