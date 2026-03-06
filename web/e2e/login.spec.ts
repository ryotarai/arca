import { expect, test } from '@playwright/test'
import { execFileSync } from 'node:child_process'
import { adminEmail, adminPassword, bestEffortDeleteMachine, ensureSetupAdmin, loginAsAdmin, waitForMachineByName } from './helpers'

const e2eDBPath = '/tmp/arca-e2e.db'

function ensureLibvirtRuntimeInDB() {
  for (let attempt = 0; attempt < 6; attempt += 1) {
    try {
      execFileSync('sqlite3', [
        e2eDBPath,
        `PRAGMA busy_timeout = 5000;
INSERT OR IGNORE INTO runtimes (id, name, type, config_json, created_at, updated_at) VALUES ('libvirt', 'libvirt-default', 'libvirt', '{"libvirt":{"uri":"qemu:///system","network":"default","storagePool":"default"}}', CAST(strftime('%s','now') AS INTEGER), CAST(strftime('%s','now') AS INTEGER));`,
      ], { stdio: 'pipe' })
      return
    } catch (error) {
      if (!String(error).includes('database is locked') || attempt === 5) {
        throw error
      }
    }
  }
}

function updateMachineForRestartVisibility(machineID: string, status: string) {
  for (let attempt = 0; attempt < 6; attempt += 1) {
    try {
      execFileSync(
        'sqlite3',
        [
          e2eDBPath,
          `PRAGMA busy_timeout = 5000;
UPDATE machines SET setup_version = 'legacy-version' WHERE id = '${machineID}';
UPDATE machine_states SET status = '${status}' WHERE machine_id = '${machineID}';`,
        ],
        { stdio: 'pipe' },
      )
      return
    } catch (error) {
      if (!String(error).includes('database is locked') || attempt === 5) {
        throw error
      }
    }
  }
}

async function createMachineViaAPI(page: import('@playwright/test').Page, machineName: string): Promise<string> {
  ensureLibvirtRuntimeInDB()

  const response = await page.request.post('/arca.v1.MachineService/CreateMachine', {
    data: { name: machineName, runtimeId: 'libvirt' },
  })
  expect(response.ok()).toBeTruthy()
  const payload = (await response.json()) as { machine?: { id?: string } }
  const machineID = payload.machine?.id?.trim() ?? ''
  expect(machineID).not.toBe('')
  return machineID
}

async function setDisableInternetPublicExposure(page: import('@playwright/test').Page, disabled: boolean): Promise<void> {
  const response = await page.request.post('/arca.v1.SetupService/UpdateDomainSettings', {
    data: {
      baseDomain: 'example.com',
      domainPrefix: 'arca-',
      disableInternetPublicExposure: disabled,
      oidcEnabled: false,
      oidcIssuerUrl: '',
      oidcClientId: '',
      oidcClientSecret: '',
      oidcAllowedEmailDomains: [],
      clearOidcClientSecret: false,
    },
  })
  expect(response.ok()).toBeTruthy()
}

test('redirect path exposes login screen', async ({ page }) => {
  await page.goto('/')

  await expect(page).toHaveURL('/login?next=%2F')
  await expect(page.getByRole('heading', { name: 'Arca' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Login' })).toBeVisible()
})

test('login route is directly accessible', async ({ page }) => {
  await page.goto('/login')

  await expect(page).toHaveURL('/login')
  await expect(page.getByLabel('Email')).toBeVisible()
  await expect(page.getByLabel('Password', { exact: true })).toBeVisible()
})

test('unauthenticated user cannot access authenticated dashboard view', async ({ page }) => {
  await page.goto('/')

  await expect(page).toHaveURL('/login?next=%2F')
  await expect(page.getByRole('button', { name: 'Login' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Logout' })).toHaveCount(0)
})

test('login and logout via Connect RPC', async ({ page }) => {
  await ensureSetupAdmin(page)

  await page.goto('/login')
  await page.getByLabel('Email').fill(adminEmail)
  await page.getByLabel('Password', { exact: true }).fill(adminPassword)

  const loginRequest = page.waitForRequest(
    (request) =>
      request.url().endsWith('/arca.v1.AuthService/Login') &&
      request.method() === 'POST',
  )
  await page.getByRole('button', { name: 'Login' }).click()
  await loginRequest

  await expect(page).toHaveURL('/machines')
  await expect(page.getByText(`Signed in as ${adminEmail}`)).toBeVisible()

  const logoutRequest = page.waitForRequest(
    (request) =>
      request.url().endsWith('/arca.v1.AuthService/Logout') &&
      request.method() === 'POST',
  )
  await page.getByRole('button', { name: 'Logout' }).first().click()
  await logoutRequest

  await expect(page).toHaveURL('/login?next=%2Fmachines')
  await expect(page.getByRole('button', { name: 'Login' })).toBeVisible()
})

test('machine CRUD screen works for authenticated user', async ({ page }) => {
  const machineName = `alpha-machine-${Date.now()}`
  await loginAsAdmin(page)
  ensureLibvirtRuntimeInDB()

  await page.getByRole('link', { name: 'Machines' }).click()
  await expect(page).toHaveURL('/machines')
  await expect(page.getByRole('heading', { name: 'Machines' })).toBeVisible()

  await page.getByRole('link', { name: 'Create machine' }).click()
  await expect(page).toHaveURL('/machines/create')
  await expect(page.getByRole('heading', { name: 'Create machine' })).toBeVisible()

  await page.getByLabel('Name').fill(machineName)
  await page.getByRole('button', { name: 'Create machine' }).click()
  await expect(page).toHaveURL(/\/machines\/.+/)
  await expect(page.getByRole('heading', { name: 'Machine detail' })).toBeVisible()
  await page.getByRole('link', { name: 'Back' }).click()
  await expect(page).toHaveURL('/machines')

  await expect(page.locator('p.font-medium', { hasText: machineName })).toBeVisible()
  await expect(page.getByText(/pending|starting|running|stopping|stopped|failed/).first()).toBeVisible()

  page.once('dialog', (dialog) => dialog.accept())
  await page.getByRole('button', { name: 'Stop' }).first().click()
  await expect(page.getByText(/stopping|stopped|failed/).first()).toBeVisible()

  await page.getByRole('link', { name: 'Details' }).first().click()
  await expect(page).toHaveURL(/\/machines\/.+/)
  await expect(page.getByRole('heading', { name: 'Machine detail' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Start', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Stop' })).toBeVisible()
  await page.getByRole('link', { name: 'Back' }).click()
  await expect(page).toHaveURL('/machines')

  page.once('dialog', (dialog) => dialog.accept())
  await page.getByRole('button', { name: 'Delete' }).first().click()
  await expect(page.getByText('No machines yet.')).toBeVisible()
})

test('machine detail runtime links to runtime detail route', async ({ page }) => {
  const machineName = `runtime-link-${Date.now()}`

  await loginAsAdmin(page)
  ensureLibvirtRuntimeInDB()

  await page.goto('/machines/create')
  await page.getByLabel('Name').fill(machineName)
  await page.getByRole('button', { name: 'Create machine' }).click()
  await expect(page).toHaveURL(/\/machines\/.+/)
  await expect(page.getByRole('heading', { name: 'Machine detail' })).toBeVisible()

  const runtimeLink = page.locator('a[href^="/runtimes/"]').first()
  await expect(runtimeLink).toBeVisible({ timeout: 15_000 })
  const runtimeHref = await runtimeLink.getAttribute('href')
  expect(runtimeHref).toBeTruthy()
  await runtimeLink.click()

  await expect(page).toHaveURL(runtimeHref!)
  await expect(page.getByRole('heading', { name: 'Runtime detail' })).toBeVisible()
  await expect(page.getByText('Runtime metadata')).toBeVisible()

  const machine = await waitForMachineByName(page, machineName)
  await bestEffortDeleteMachine(page, machine.id)
})

test('authenticated login route honors next parameter', async ({ page }) => {
  await loginAsAdmin(page)

  await page.goto('/login?next=%2Fmachines')
  await expect(page).toHaveURL('/machines')
  await expect(page.getByRole('heading', { name: 'Machines' })).toBeVisible()
})

test('authenticated login route honors nested authorize next parameter', async ({ page }) => {
  await loginAsAdmin(page)

  await page.goto(
    '/login?next=%2Fconsole%2Fauthorize%3Ftarget%3Dhttps%253A%252F%252Farca-test3.ryotarai.info%252Fcallback%253Fnext%253D%25252F',
  )
  await expect(page).toHaveURL(
    '/console/authorize?target=https%3A%2F%2Farca-test3.ryotarai.info%2Fcallback%3Fnext%3D%252F',
  )
})

test('machines list shows restart CTA only when update is required and machine is restartable', async ({ page }) => {
  const machineName = `restart-list-${Date.now()}`

  await loginAsAdmin(page)
  const machineID = await createMachineViaAPI(page, machineName)
  await waitForMachineByName(page, machineName)

  try {
    updateMachineForRestartVisibility(machineID, 'running')

    await page.goto('/machines')
    const row = page.locator('li', { hasText: machineName })
    await expect(row.getByRole('button', { name: 'Restart to update' })).toBeVisible()

    for (const status of ['starting', 'stopping', 'pending', 'deleting']) {
      updateMachineForRestartVisibility(machineID, status)
      await page.goto('/machines')
      await expect(row.getByRole('button', { name: 'Restart to update' })).toHaveCount(0)
    }
  } finally {
    await bestEffortDeleteMachine(page, machineID)
  }
})

test('machine detail shows restart CTA only when update is required and machine is restartable', async ({ page }) => {
  const machineName = `restart-detail-${Date.now()}`

  await loginAsAdmin(page)
  const machineID = await createMachineViaAPI(page, machineName)
  await waitForMachineByName(page, machineName)

  try {
    updateMachineForRestartVisibility(machineID, 'running')

    await page.goto(`/machines/${machineID}`)
    await expect(page.getByRole('button', { name: 'Restart to update' })).toBeVisible()

    for (const status of ['starting', 'stopping', 'pending', 'deleting']) {
      updateMachineForRestartVisibility(machineID, status)
      await page.goto(`/machines/${machineID}`)
      await expect(page.getByRole('button', { name: 'Restart to update' })).toHaveCount(0)
    }
  } finally {
    await bestEffortDeleteMachine(page, machineID)
  }
})

test('machine detail disables internet-public visibility when blocked by admin policy', async ({ page }) => {
  test.setTimeout(90_000)
  const machineName = `visibility-policy-${Date.now()}`

  await loginAsAdmin(page)
  const machineID = await createMachineViaAPI(page, machineName)

  try {
    await setDisableInternetPublicExposure(page, true)

    await page.goto('/machines/' + machineID + '/edit')
    await expect(page.getByRole('heading', { name: 'Edit machine' })).toBeVisible()
    await expect(page.getByText(/Internet public visibility is disabled by admin policy./)).toBeVisible()

    const internetPublicOption = page.getByRole('option', { name: 'Internet public' })
    await expect(internetPublicOption).toHaveAttribute('disabled', '')
  } finally {
    await setDisableInternetPublicExposure(page, false)
    await bestEffortDeleteMachine(page, machineID)
  }
})

test('runtime catalog CRUD enforces typed config form constraints', async ({ page }) => {
  const runtimeName = `runtime-${Date.now()}`

  await loginAsAdmin(page)
  await page.getByRole('link', { name: 'Runtimes' }).click()
  await expect(page).toHaveURL('/runtimes')
  await expect(page.getByRole('heading', { name: 'Runtimes' })).toBeVisible()

  const submitButton = page.getByRole('button', { name: 'Create runtime' })
  await expect(submitButton).toBeDisabled()

  await page.getByLabel('Name').fill(runtimeName)
  await expect(submitButton).toBeDisabled()

  await page.getByLabel('URI').fill('qemu:///system')
  await page.getByLabel('Network').first().fill('default')
  await page.getByLabel('Storage pool').fill('default')
  await expect(submitButton).toBeEnabled()
  await submitButton.click()

  const row = page.locator('div.rounded-lg', { hasText: runtimeName })
  await expect(row).toBeVisible()

  await row.getByRole('button', { name: 'Edit' }).click()
  await page.getByLabel('Type').selectOption('gce')
  await expect(page.getByRole('button', { name: 'Save runtime' })).toBeDisabled()

  await page.getByLabel('Project').fill('my-project')
  await page.getByLabel('Zone').fill('us-central1-a')
  await page.getByLabel('Network').first().fill('vpc-main')
  await page.getByLabel('Subnetwork').fill('subnet-main')
  await page.getByLabel('Service account email').fill('svc@example.iam.gserviceaccount.com')
  await page.getByRole('button', { name: 'Save runtime' }).click()
  await expect(row.getByText('gce')).toBeVisible()

  page.once('dialog', (dialog) => dialog.accept())
  await row.getByRole('button', { name: 'Delete' }).click()
  await expect(row).toHaveCount(0)
})
