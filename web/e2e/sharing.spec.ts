import { expect, test } from '@playwright/test'
import {
  completeUserSetup,
  createAuthedUserContext,
  createSecondaryUser,
  loginAsAdmin,
} from './helpers/auth'
import { bestEffortDeleteMachine, createMachineViaAPI } from './helpers/machine'
import { ensureLxdRuntime } from './helpers/runtime'

test.describe('sharing', () => {
  test('Share button opens sharing dialog', async ({ page }) => {
    test.setTimeout(90_000)
    await loginAsAdmin(page)
    const runtime = await ensureLxdRuntime(page)
    const machineName = `share-dialog-${Date.now()}`
    const machineID = await createMachineViaAPI(page, machineName, runtime.id)

    try {
      await page.goto(`/machines/${machineID}`)
      await expect(page.getByRole('heading', { name: 'Machine detail' })).toBeVisible()
      await page.getByRole('button', { name: 'Share' }).click()
      await expect(page.getByText('Share machine')).toBeVisible()
      await expect(page.getByText('General access')).toBeVisible()
    } finally {
      await bestEffortDeleteMachine(page, machineID)
    }
  })

  test('add member by email', async ({ page }) => {
    test.setTimeout(90_000)
    await loginAsAdmin(page)
    const runtime = await ensureLxdRuntime(page)
    const machineName = `share-add-${Date.now()}`
    const machineID = await createMachineViaAPI(page, machineName, runtime.id)
    const memberEmail = `member-${Date.now()}@example.com`
    await createSecondaryUser(page, memberEmail)

    try {
      await page.goto(`/machines/${machineID}`)
      await page.getByRole('button', { name: 'Share' }).click()
      await expect(page.getByText('Share machine')).toBeVisible()

      await page.getByPlaceholder('Add by email').fill(memberEmail)
      await page.getByRole('button', { name: 'Add' }).click()

      await expect(page.getByText(memberEmail)).toBeVisible()
    } finally {
      await bestEffortDeleteMachine(page, machineID)
    }
  })

  test('change member role', async ({ page }) => {
    test.setTimeout(90_000)
    await loginAsAdmin(page)
    const runtime = await ensureLxdRuntime(page)
    const machineName = `share-role-${Date.now()}`
    const machineID = await createMachineViaAPI(page, machineName, runtime.id)
    const memberEmail = `role-member-${Date.now()}@example.com`
    await createSecondaryUser(page, memberEmail)

    try {
      await page.goto(`/machines/${machineID}`)
      await page.getByRole('button', { name: 'Share' }).click()
      await expect(page.getByText('Share machine')).toBeVisible()

      await page.getByPlaceholder('Add by email').fill(memberEmail)
      await page.getByRole('button', { name: 'Add' }).click()
      await expect(page.getByText(memberEmail)).toBeVisible()

      // Find the member row by its containing element and change role
      const dialog = page.locator('[role="dialog"]')
      const memberRow = dialog.locator('.rounded-lg.border', { hasText: memberEmail })
      await memberRow.locator('select').selectOption('editor')

      // Save and verify
      await dialog.getByRole('button', { name: 'Save' }).click()
    } finally {
      await bestEffortDeleteMachine(page, machineID)
    }
  })

  test('remove member', async ({ page }) => {
    test.setTimeout(90_000)
    await loginAsAdmin(page)
    const runtime = await ensureLxdRuntime(page)
    const machineName = `share-rm-${Date.now()}`
    const machineID = await createMachineViaAPI(page, machineName, runtime.id)
    const memberEmail = `rm-member-${Date.now()}@example.com`
    await createSecondaryUser(page, memberEmail)

    try {
      await page.goto(`/machines/${machineID}`)
      await page.getByRole('button', { name: 'Share' }).click()
      await expect(page.getByText('Share machine')).toBeVisible()

      await page.getByPlaceholder('Add by email').fill(memberEmail)
      await page.getByRole('button', { name: 'Add' }).click()
      await expect(page.getByText(memberEmail)).toBeVisible()

      // Remove the member
      const dialog = page.locator('[role="dialog"]')
      const memberRow = dialog.locator('.rounded-lg.border', { hasText: memberEmail })
      await memberRow.getByRole('button', { name: 'Remove' }).click()

      // Save
      await dialog.getByRole('button', { name: 'Save' }).click()
    } finally {
      await bestEffortDeleteMachine(page, machineID)
    }
  })

  test('toggle general access', async ({ page }) => {
    test.setTimeout(90_000)
    await loginAsAdmin(page)
    const runtime = await ensureLxdRuntime(page)
    const machineName = `share-gen-${Date.now()}`
    const machineID = await createMachineViaAPI(page, machineName, runtime.id)

    try {
      await page.goto(`/machines/${machineID}`)
      await page.getByRole('button', { name: 'Share' }).click()
      await expect(page.getByText('Share machine')).toBeVisible()

      // The general access select is in the dialog - use the specific one after "General access" text
      const dialog = page.locator('[role="dialog"]')
      // The general access select has options "Restricted" and "Anyone with an Arca account"
      // It's the select NOT inside a member row
      const generalAccessSelect = dialog
        .locator('select')
        .filter({ hasText: 'Restricted' })
      await generalAccessSelect.selectOption('arca_users')

      await dialog.getByRole('button', { name: 'Save' }).click()

      // Reopen and verify
      await page.getByRole('button', { name: 'Share' }).click()
      await expect(page.getByText('Share machine')).toBeVisible()
      await expect(
        dialog.getByText('Any authenticated Arca user can view this machine'),
      ).toBeVisible()
    } finally {
      await bestEffortDeleteMachine(page, machineID)
    }
  })

  test('sharing persists after dialog close and reopen', async ({ page }) => {
    test.setTimeout(90_000)
    await loginAsAdmin(page)
    const runtime = await ensureLxdRuntime(page)
    const machineName = `share-persist-${Date.now()}`
    const machineID = await createMachineViaAPI(page, machineName, runtime.id)
    const memberEmail = `persist-member-${Date.now()}@example.com`
    await createSecondaryUser(page, memberEmail)

    try {
      await page.goto(`/machines/${machineID}`)
      await page.getByRole('button', { name: 'Share' }).click()
      await expect(page.getByText('Share machine')).toBeVisible()

      const dialog = page.locator('[role="dialog"]')
      await page.getByPlaceholder('Add by email').fill(memberEmail)
      await dialog.getByRole('button', { name: 'Add' }).click()

      // Verify member appears in the members list (use the member row, not search dropdown)
      const memberRow = dialog.locator('.rounded-lg.border', { hasText: memberEmail })
      await expect(memberRow).toBeVisible()

      await dialog.getByRole('button', { name: 'Save' }).click()

      // Reopen dialog and verify member is still there
      await page.getByRole('button', { name: 'Share' }).click()
      await expect(page.getByText('Share machine')).toBeVisible()
      const memberRowAfter = dialog.locator('.rounded-lg.border', { hasText: memberEmail })
      await expect(memberRowAfter).toBeVisible()
    } finally {
      await bestEffortDeleteMachine(page, machineID)
    }
  })

  test('non-admin shared user cannot see admin controls', async ({ page, browser }) => {
    test.setTimeout(90_000)
    await loginAsAdmin(page)
    const runtime = await ensureLxdRuntime(page)
    const machineName = `share-nonadmin-${Date.now()}`
    const machineID = await createMachineViaAPI(page, machineName, runtime.id)
    const viewerEmail = `viewer-${Date.now()}@example.com`
    const viewerPassword = 'password456'
    const { setupToken } = await createSecondaryUser(page, viewerEmail)
    await completeUserSetup(page, setupToken, viewerPassword)

    // Share with the viewer via API
    await page.request.post('/arca.v1.SharingService/UpdateMachineSharing', {
      data: {
        machineId: machineID,
        members: [{ email: viewerEmail, role: 'viewer' }],
        generalAccess: { scope: 'none', role: 'none' },
      },
    })

    try {
      const ctx = await createAuthedUserContext(
        browser,
        process.env.PLAYWRIGHT_BASE_URL ?? 'http://127.0.0.1:18080',
        viewerEmail,
        viewerPassword,
      )
      try {
        await ctx.page.goto(`/machines/${machineID}`)
        await expect(ctx.page.getByRole('heading', { name: 'Machine detail' })).toBeVisible()
        // Viewer should NOT see admin controls
        await expect(
          ctx.page.getByRole('button', { name: 'Start', exact: true }),
        ).toHaveCount(0)
        await expect(ctx.page.getByRole('button', { name: 'Stop' })).toHaveCount(0)
        await expect(ctx.page.getByRole('button', { name: 'Delete' })).toHaveCount(0)
        await expect(ctx.page.getByRole('button', { name: 'Share' })).toHaveCount(0)
      } finally {
        await ctx.close()
      }
    } finally {
      await bestEffortDeleteMachine(page, machineID)
    }
  })
})
