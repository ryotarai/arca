import { expect, test } from '@playwright/test'
import { loginAsAdmin } from './helpers/auth'
import { createMachineProfileViaAPI } from './helpers/machine-profile'

test.describe('machine options', () => {
  test('create machine with options via API and read them back', async ({ page }) => {
    await loginAsAdmin(page)

    // Create a GCE runtime with allowed machine types
    const runtime = await createMachineProfileViaAPI(page, {
      name: `gce-opts-${Date.now()}`,
      type: 'gce',
      config: {
        gce: {
          project: 'test-project',
          zone: 'us-central1-a',
          network: 'default',
          subnetwork: 'default',
          serviceAccountEmail: 'test@test.iam.gserviceaccount.com',
          allowedMachineTypes: ['e2-medium', 'e2-standard-2', 'e2-standard-4'],
        },
        exposure: {
          method: 2,
          connectivity: 1,
        },
      },
    })

    // Create machine with machine_type option
    const createResp = await page.request.post('/arca.v1.MachineService/CreateMachine', {
      data: {
        name: `opts-test-${Date.now()}`,
        profileId: runtime.id,
        options: { machine_type: 'e2-medium' },
      },
    })
    expect(createResp.ok()).toBeTruthy()
    const createPayload = (await createResp.json()) as {
      machine?: { id?: string; options?: Record<string, string> }
    }
    const machineID = createPayload.machine?.id ?? ''
    expect(machineID).not.toBe('')
    expect(createPayload.machine?.options?.machine_type).toBe('e2-medium')

    // Get machine and verify options are returned
    const getResp = await page.request.post('/arca.v1.MachineService/GetMachine', {
      data: { machineId: machineID },
    })
    expect(getResp.ok()).toBeTruthy()
    const getPayload = (await getResp.json()) as {
      machine?: { options?: Record<string, string> }
    }
    expect(getPayload.machine?.options?.machine_type).toBe('e2-medium')
  })

  test('update machine options requires stopped status', async ({ page }) => {
    await loginAsAdmin(page)

    const runtime = await createMachineProfileViaAPI(page, {
      name: `gce-upd-${Date.now()}`,
      type: 'gce',
      config: {
        gce: {
          project: 'test-project',
          zone: 'us-central1-a',
          network: 'default',
          subnetwork: 'default',
          serviceAccountEmail: 'test@test.iam.gserviceaccount.com',
          allowedMachineTypes: ['e2-medium', 'e2-standard-2'],
        },
        exposure: {
          method: 2,
          connectivity: 1,
        },
      },
    })

    // Create machine (starts in pending state)
    const createResp = await page.request.post('/arca.v1.MachineService/CreateMachine', {
      data: {
        name: `upd-test-${Date.now()}`,
        profileId: runtime.id,
        options: { machine_type: 'e2-standard-2' },
      },
    })
    expect(createResp.ok()).toBeTruthy()
    const createPayload = (await createResp.json()) as {
      machine?: { id?: string }
    }
    const machineID = createPayload.machine?.id ?? ''

    // Try to update options on a non-stopped machine - should fail
    const updateResp = await page.request.post('/arca.v1.MachineService/UpdateMachine', {
      data: {
        machineId: machineID,
        options: { machine_type: 'e2-medium' },
      },
      failOnStatusCode: false,
    })
    expect(updateResp.ok()).toBeFalsy()
    const errorPayload = (await updateResp.json()) as { message?: string }
    expect(errorPayload.message).toContain('stopped')
  })

  test('create machine rejects invalid machine type', async ({ page }) => {
    await loginAsAdmin(page)

    const runtime = await createMachineProfileViaAPI(page, {
      name: `gce-inv-${Date.now()}`,
      type: 'gce',
      config: {
        gce: {
          project: 'test-project',
          zone: 'us-central1-a',
          network: 'default',
          subnetwork: 'default',
          serviceAccountEmail: 'test@test.iam.gserviceaccount.com',
          allowedMachineTypes: ['e2-medium', 'e2-standard-2'],
        },
        exposure: {
          method: 2,
          connectivity: 1,
        },
      },
    })

    // Verify the profile config was stored with allowedMachineTypes
    const listResp = await page.request.post('/arca.v1.MachineProfileService/ListMachineProfiles', { data: {} })
    const listPayload = (await listResp.json()) as {
      profiles?: Array<{ id?: string; config?: { gce?: { allowedMachineTypes?: string[] } } }>
    }
    const storedProfile = listPayload.profiles?.find((r) => r.id === runtime.id)
    expect(storedProfile?.config?.gce?.allowedMachineTypes).toEqual(['e2-medium', 'e2-standard-2'])

    const resp = await page.request.post('/arca.v1.MachineService/CreateMachine', {
      data: {
        name: `inv-test-${Date.now()}`,
        profileId: runtime.id,
        options: { machine_type: 'n1-standard-96' },
      },
      failOnStatusCode: false,
    })
    expect(resp.ok()).toBeFalsy()
    const payload = (await resp.json()) as { message?: string }
    expect(payload.message).toContain('not allowed')
  })

  test('create machine rejects empty allowedMachineTypes in profile config', async ({ page }) => {
    await loginAsAdmin(page)

    const resp = await page.request.post('/arca.v1.MachineProfileService/CreateMachineProfile', {
      data: {
        name: `gce-empty-mt-${Date.now()}`,
        type: 2,
        config: {
          gce: {
            project: 'test-project',
            zone: 'us-central1-a',
            network: 'default',
            subnetwork: 'default',
            serviceAccountEmail: 'test@test.iam.gserviceaccount.com',
          },
          exposure: {
            method: 2,
            connectivity: 1,
          },
        },
      },
      failOnStatusCode: false,
    })
    expect(resp.ok()).toBeFalsy()
    const payload = (await resp.json()) as { message?: string }
    expect(payload.message).toContain('allowed machine type')
  })

  test('create machine form shows machine type for GCE profile', async ({ page }) => {
    await loginAsAdmin(page)

    const runtime = await createMachineProfileViaAPI(page, {
      name: `gce-form-${Date.now()}`,
      type: 'gce',
      config: {
        gce: {
          project: 'test-project',
          zone: 'us-central1-a',
          network: 'default',
          subnetwork: 'default',
          serviceAccountEmail: 'test@test.iam.gserviceaccount.com',
          allowedMachineTypes: ['e2-medium', 'e2-standard-2'],
        },
        exposure: {
          method: 2,
          connectivity: 1,
        },
      },
    })

    await page.goto('/machines/create')
    await page.waitForSelector('#create-machine-profile')

    // Select the GCE profile
    await page.locator('#create-machine-profile').click()
    await page.getByRole('option', { name: new RegExp(runtime.name) }).click()

    // Machine type selector should appear
    const machineTypeField = page.locator('#create-machine-type')
    await expect(machineTypeField).toBeVisible({ timeout: 5000 })
  })

  test('profile form has allowed machine types field for GCE', async ({ page }) => {
    await loginAsAdmin(page)

    await page.goto('/machine-profiles/new')
    await page.waitForSelector('#profile-type')

    // Select GCE type
    await page.locator('#profile-type').click()
    await page.getByRole('option', { name: 'Google Compute Engine (GCE)' }).click()

    // Check allowed machine types field is visible
    const allowedField = page.locator('#profile-gce-allowed-machine-types')
    await expect(allowedField).toBeVisible()
  })
})
