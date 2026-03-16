import { expect, test } from '@playwright/test'
import { loginAsAdmin } from './helpers/auth'
import { createRuntimeViaAPI } from './helpers/runtime'

test.describe('machine options', () => {
  test('create machine with options via API and read them back', async ({ page }) => {
    await loginAsAdmin(page)

    // Create a GCE runtime with allowed machine types
    const runtime = await createRuntimeViaAPI(page, {
      name: `gce-opts-${Date.now()}`,
      type: 'gce',
      config: {
        gce: {
          project: 'test-project',
          zone: 'us-central1-a',
          network: 'default',
          subnetwork: 'default',
          serviceAccountEmail: 'test@test.iam.gserviceaccount.com',
          machineType: 'e2-standard-2',
          allowedMachineTypes: ['e2-medium', 'e2-standard-2', 'e2-standard-4'],
        },
        exposure: {
          method: 2,
          domainPrefix: 'arca-',
          baseDomain: 'localhost',
          connectivity: 1,
        },
      },
    })

    // Create machine with machine_type option
    const createResp = await page.request.post('/arca.v1.MachineService/CreateMachine', {
      data: {
        name: `opts-test-${Date.now()}`,
        runtimeId: runtime.id,
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

    const runtime = await createRuntimeViaAPI(page, {
      name: `gce-upd-${Date.now()}`,
      type: 'gce',
      config: {
        gce: {
          project: 'test-project',
          zone: 'us-central1-a',
          network: 'default',
          subnetwork: 'default',
          serviceAccountEmail: 'test@test.iam.gserviceaccount.com',
          machineType: 'e2-standard-2',
          allowedMachineTypes: ['e2-medium', 'e2-standard-2'],
        },
        exposure: {
          method: 2,
          domainPrefix: 'arca-',
          baseDomain: 'localhost',
          connectivity: 1,
        },
      },
    })

    // Create machine (starts in pending state)
    const createResp = await page.request.post('/arca.v1.MachineService/CreateMachine', {
      data: {
        name: `upd-test-${Date.now()}`,
        runtimeId: runtime.id,
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

    const runtime = await createRuntimeViaAPI(page, {
      name: `gce-inv-${Date.now()}`,
      type: 'gce',
      config: {
        gce: {
          project: 'test-project',
          zone: 'us-central1-a',
          network: 'default',
          subnetwork: 'default',
          serviceAccountEmail: 'test@test.iam.gserviceaccount.com',
          machineType: 'e2-standard-2',
          allowedMachineTypes: ['e2-medium', 'e2-standard-2'],
        },
        exposure: {
          method: 2,
          domainPrefix: 'arca-',
          baseDomain: 'localhost',
          connectivity: 1,
        },
      },
    })

    // Verify the runtime config was stored with allowedMachineTypes
    const listResp = await page.request.post('/arca.v1.RuntimeService/ListRuntimes', { data: {} })
    const listPayload = (await listResp.json()) as {
      runtimes?: Array<{ id?: string; config?: { gce?: { allowedMachineTypes?: string[] } } }>
    }
    const storedRuntime = listPayload.runtimes?.find((r) => r.id === runtime.id)
    expect(storedRuntime?.config?.gce?.allowedMachineTypes).toEqual(['e2-medium', 'e2-standard-2'])

    const resp = await page.request.post('/arca.v1.MachineService/CreateMachine', {
      data: {
        name: `inv-test-${Date.now()}`,
        runtimeId: runtime.id,
        options: { machine_type: 'n1-standard-96' },
      },
      failOnStatusCode: false,
    })
    expect(resp.ok()).toBeFalsy()
    const payload = (await resp.json()) as { message?: string }
    expect(payload.message).toContain('not allowed')
  })

  test('create machine allows any type when allowedMachineTypes is empty', async ({ page }) => {
    await loginAsAdmin(page)

    const runtime = await createRuntimeViaAPI(page, {
      name: `gce-any-${Date.now()}`,
      type: 'gce',
      config: {
        gce: {
          project: 'test-project',
          zone: 'us-central1-a',
          network: 'default',
          subnetwork: 'default',
          serviceAccountEmail: 'test@test.iam.gserviceaccount.com',
          machineType: 'e2-standard-2',
        },
        exposure: {
          method: 2,
          domainPrefix: 'arca-',
          baseDomain: 'localhost',
          connectivity: 1,
        },
      },
    })

    const resp = await page.request.post('/arca.v1.MachineService/CreateMachine', {
      data: {
        name: `any-test-${Date.now()}`,
        runtimeId: runtime.id,
        options: { machine_type: 'n1-standard-96' },
      },
    })
    expect(resp.ok()).toBeTruthy()
    const payload = (await resp.json()) as {
      machine?: { options?: Record<string, string> }
    }
    expect(payload.machine?.options?.machine_type).toBe('n1-standard-96')
  })

  test('create machine form shows machine type for GCE runtime', async ({ page }) => {
    await loginAsAdmin(page)

    const runtime = await createRuntimeViaAPI(page, {
      name: `gce-form-${Date.now()}`,
      type: 'gce',
      config: {
        gce: {
          project: 'test-project',
          zone: 'us-central1-a',
          network: 'default',
          subnetwork: 'default',
          serviceAccountEmail: 'test@test.iam.gserviceaccount.com',
          machineType: 'e2-standard-2',
          allowedMachineTypes: ['e2-medium', 'e2-standard-2'],
        },
        exposure: {
          method: 2,
          domainPrefix: 'arca-',
          baseDomain: 'localhost',
          connectivity: 1,
        },
      },
    })

    await page.goto('/machines/create')
    await page.waitForSelector('#create-machine-runtime')

    // Select the GCE runtime
    await page.selectOption('#create-machine-runtime', runtime.id)

    // Machine type selector should appear
    const machineTypeField = page.locator('#create-machine-type')
    await expect(machineTypeField).toBeVisible({ timeout: 5000 })
  })

  test('runtime form has allowed machine types field for GCE', async ({ page }) => {
    await loginAsAdmin(page)

    await page.goto('/runtimes/new')
    await page.waitForSelector('#runtime-type')

    // Select GCE type
    await page.selectOption('#runtime-type', 'gce')

    // Check allowed machine types field is visible
    const allowedField = page.locator('#runtime-gce-allowed-machine-types')
    await expect(allowedField).toBeVisible()
  })
})
