import { expect, test } from '@playwright/test'
import {
  createSecondaryUser,
  loginAsAdmin,
} from './helpers/auth'
import { bestEffortDeleteMachine, createMachineViaAPI } from './helpers/machine'
import { ensureLxdRuntime } from './helpers/runtime'

async function createGroupViaAPI(page: import('@playwright/test').Page, name: string): Promise<string> {
  const response = await page.request.post('/arca.v1.GroupService/CreateGroup', {
    data: { name },
  })
  expect(response.ok()).toBeTruthy()
  const payload = (await response.json()) as { group?: { id?: string } }
  const groupId = payload.group?.id?.trim() ?? ''
  expect(groupId).not.toBe('')
  return groupId
}

async function deleteGroupViaAPI(page: import('@playwright/test').Page, groupId: string) {
  await page.request.post('/arca.v1.GroupService/DeleteGroup', {
    data: { groupId },
    failOnStatusCode: false,
  })
}

test.describe('groups', () => {
  test('admin can see Groups link in sidebar', async ({ page }) => {
    await loginAsAdmin(page)
    await expect(page.getByRole('link', { name: 'Groups' })).toBeVisible()
  })

  test('admin can navigate to groups page', async ({ page }) => {
    await loginAsAdmin(page)
    await page.getByRole('link', { name: 'Groups' }).click()
    await expect(page).toHaveURL('/groups')
    await expect(page.getByRole('heading', { name: 'Groups' })).toBeVisible()
  })

  test('admin can create a group', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/groups')
    await expect(page.getByRole('heading', { name: 'Groups' })).toBeVisible()

    const groupName = `test-group-${Date.now()}`
    await page.getByLabel('Name').fill(groupName)
    await page.getByRole('button', { name: 'Create group' }).click()

    // Verify group appears in the list
    await expect(page.getByText(groupName)).toBeVisible()

    // Clean up
    const groups = await page.request.post('/arca.v1.GroupService/ListGroups', { data: {} })
    const payload = (await groups.json()) as { groups?: { id: string; name: string }[] }
    const group = payload.groups?.find((g) => g.name === groupName)
    if (group) {
      await deleteGroupViaAPI(page, group.id)
    }
  })

  test('admin can delete a group', async ({ page }) => {
    await loginAsAdmin(page)
    const groupName = `del-group-${Date.now()}`
    const groupId = await createGroupViaAPI(page, groupName)

    await page.goto('/groups')
    await expect(page.getByText(groupName)).toBeVisible()

    // Click delete button in the group row
    const groupRow = page.locator('.rounded-lg.border', { hasText: groupName })
    await groupRow.getByRole('button', { name: 'Delete' }).click()

    // Verify group is removed
    await expect(page.getByText(groupName)).toHaveCount(0)

    // Clean up just in case
    await deleteGroupViaAPI(page, groupId)
  })

  test('admin can add and remove members via group detail', async ({ page }) => {
    test.setTimeout(60_000)
    await loginAsAdmin(page)

    const groupName = `member-group-${Date.now()}`
    const groupId = await createGroupViaAPI(page, groupName)
    const memberEmail = `grp-member-${Date.now()}@example.com`
    const { userId } = await createSecondaryUser(page, memberEmail)

    try {
      // Add member via API
      const addResp = await page.request.post('/arca.v1.GroupService/AddGroupMember', {
        data: { groupId, userId },
      })
      expect(addResp.ok()).toBeTruthy()

      // Navigate to groups page and click the group
      await page.goto('/groups')
      await expect(page.getByText(groupName)).toBeVisible()
      await page.locator('.rounded-lg.border', { hasText: groupName }).click()

      // Verify member appears in the detail section
      await expect(page.getByText(memberEmail)).toBeVisible()

      // Remove member
      const memberRow = page.locator('.rounded-lg.border', { hasText: memberEmail })
      await memberRow.getByRole('button', { name: 'Remove' }).click()

      // Verify member is removed
      await expect(page.getByText(memberEmail)).toHaveCount(0)
    } finally {
      await deleteGroupViaAPI(page, groupId)
    }
  })

  test('group sharing via API', async ({ page }) => {
    test.setTimeout(90_000)
    await loginAsAdmin(page)

    const runtime = await ensureLxdRuntime(page)
    const groupName = `share-group-${Date.now()}`
    const groupId = await createGroupViaAPI(page, groupName)
    const machineId = await createMachineViaAPI(page, `share-test-${Date.now()}`, runtime.id)

    try {
      // Share machine with group via API
      const shareResp = await page.request.post('/arca.v1.SharingService/UpdateMachineSharing', {
        data: {
          machineId,
          members: [],
          generalAccess: { scope: 'none', role: 'none' },
          groups: [{ groupId, name: groupName, role: 'viewer' }],
        },
      })
      expect(shareResp.ok()).toBeTruthy()
      const sharePayload = (await shareResp.json()) as { groups?: { groupId: string; role: string }[] }
      expect(sharePayload.groups).toHaveLength(1)
      expect(sharePayload.groups?.[0]?.groupId).toBe(groupId)
      expect(sharePayload.groups?.[0]?.role).toBe('viewer')

      // Get sharing and verify groups are returned
      const getResp = await page.request.post('/arca.v1.SharingService/GetMachineSharing', {
        data: { machineId },
      })
      expect(getResp.ok()).toBeTruthy()
      const getPayload = (await getResp.json()) as { groups?: { groupId: string; role: string }[] }
      expect(getPayload.groups).toHaveLength(1)
      expect(getPayload.groups?.[0]?.groupId).toBe(groupId)
    } finally {
      await bestEffortDeleteMachine(page, machineId)
      await deleteGroupViaAPI(page, groupId)
    }
  })
})
