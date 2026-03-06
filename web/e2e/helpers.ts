import { expect, type APIRequestContext, type APIResponse, type Page, test } from '@playwright/test'

export const adminEmail = 'admin@example.com'
export const adminPassword = 'password123'

const defaultSetupConfig = {
  cloudflareToken: 'dummy-token',
  cloudflareAccountID: 'dummy-account',
  cloudflareZoneID: 'dummy-zone',
  baseDomain: 'example.com',
  domainPrefix: 'arca-',
}

const requiredCloudflareEnvVarNames = [
  'CLOUDFLARE_TOKEN',
  'CLOUDFLARE_ACCOUNT_ID',
  'CLOUDFLARE_ZONE_ID',
  'BASE_DOMAIN',
  'DOMAIN_PREFIX',
] as const

type CloudflareConfig = {
  cloudflareToken: string
  cloudflareAccountID: string
  cloudflareZoneID: string
  baseDomain: string
  domainPrefix: string
}

type MachineRecord = {
  id: string
  name: string
  status: string
  desiredStatus: string
  endpoint?: string
}

type PollOptions = {
  timeoutMs?: number
  intervalMs?: number
}

function trimEnv(value: string | undefined): string {
  return value?.trim() ?? ''
}

async function parseJSONSafe(response: APIResponse): Promise<Record<string, unknown> | null> {
  try {
    return (await response.json()) as Record<string, unknown>
  } catch {
    return null
  }
}

function cloudflareEnvConfig(): { config: CloudflareConfig | null; missing: string[] } {
  const config: CloudflareConfig = {
    cloudflareToken: trimEnv(process.env.CLOUDFLARE_TOKEN),
    cloudflareAccountID: trimEnv(process.env.CLOUDFLARE_ACCOUNT_ID),
    cloudflareZoneID: trimEnv(process.env.CLOUDFLARE_ZONE_ID),
    baseDomain: trimEnv(process.env.BASE_DOMAIN).toLowerCase(),
    domainPrefix: trimEnv(process.env.DOMAIN_PREFIX).toLowerCase(),
  }

  const missing: string[] = []
  for (const key of requiredCloudflareEnvVarNames) {
    if (trimEnv(process.env[key]) === '') {
      missing.push(key)
    }
  }

  if (missing.length > 0) {
    return { config: null, missing }
  }
  return { config, missing: [] }
}

export function cloudflareIntegrationConfig(): { config: CloudflareConfig | null; missing: string[] } {
  return cloudflareEnvConfig()
}

export function skipCloudflareIntegrationIfMissing() {
  const { missing } = cloudflareEnvConfig()
  const guidance =
    'Cloudflare integration E2E requires env vars: CLOUDFLARE_TOKEN, CLOUDFLARE_ACCOUNT_ID, CLOUDFLARE_ZONE_ID, BASE_DOMAIN, DOMAIN_PREFIX'
  test.skip(missing.length > 0, `${guidance}. Missing: ${missing.join(', ')}`)
}

export async function ensureSetupAdmin(page: Page) {
  const envConfig = cloudflareEnvConfig().config
  const setupConfig = envConfig ?? defaultSetupConfig

  const response = await page.request.post('/arca.v1.SetupService/CompleteSetup', {
    data: {
      adminEmail,
      adminPassword,
      baseDomain: setupConfig.baseDomain,
      domainPrefix: setupConfig.domainPrefix,
      cloudflareApiToken: setupConfig.cloudflareToken,
      cloudflareZoneId: setupConfig.cloudflareZoneID,
    },
  })

  if (response.ok()) {
    return
  }

  const payload = await parseJSONSafe(response)
  const code = String(payload?.code ?? '').toLowerCase()
  if (code === 'failed_precondition' || code === 'already_exists') {
    return
  }

  throw new Error(`setup failed: ${response.status()} ${JSON.stringify(payload)}`)
}

export async function loginAsAdmin(page: Page) {
  await ensureSetupAdmin(page)
  await page.goto('/login')
  await page.getByLabel('Email').fill(adminEmail)
  await page.getByLabel('Password', { exact: true }).fill(adminPassword)
  await page.getByRole('button', { name: 'Login' }).click()
  await expect(page).toHaveURL('/machines')
}

async function sleep(ms: number) {
  await new Promise<void>((resolve) => {
    setTimeout(resolve, ms)
  })
}

function asMachineRecord(input: unknown): MachineRecord | null {
  if (input == null || typeof input !== 'object') {
    return null
  }
  const value = input as Record<string, unknown>
  if (
    typeof value.id !== 'string' ||
    typeof value.name !== 'string' ||
    typeof value.status !== 'string' ||
    typeof value.desiredStatus !== 'string'
  ) {
    return null
  }

  return {
    id: value.id,
    name: value.name,
    status: value.status,
    desiredStatus: value.desiredStatus,
    endpoint: typeof value.endpoint === 'string' ? value.endpoint : undefined,
  }
}

async function postJSON(
  request: APIRequestContext,
  path: string,
  data: Record<string, unknown>,
): Promise<Record<string, unknown>> {
  const response = await request.post(path, { data })
  const payload = await parseJSONSafe(response)
  if (!response.ok()) {
    throw new Error(`request failed ${path}: ${response.status()} ${JSON.stringify(payload)}`)
  }
  return payload ?? {}
}

async function poll<T>(fn: () => Promise<T>, isDone: (value: T) => boolean, options: PollOptions = {}): Promise<T> {
  const timeoutMs = options.timeoutMs ?? 5 * 60 * 1000
  const intervalMs = options.intervalMs ?? 3000
  const deadline = Date.now() + timeoutMs
  let lastValue: T | null = null
  let lastError: unknown = null

  while (Date.now() <= deadline) {
    try {
      const value = await fn()
      lastValue = value
      if (isDone(value)) {
        return value
      }
    } catch (error) {
      lastError = error
    }
    await sleep(intervalMs)
  }

  const tail = lastError != null ? `lastError=${String(lastError)}` : `lastValue=${JSON.stringify(lastValue)}`
  throw new Error(`poll timeout after ${timeoutMs}ms (${tail})`)
}

export async function waitForMachineByName(page: Page, name: string, options: PollOptions = {}): Promise<MachineRecord> {
  return poll(
    async () => {
      const payload = await postJSON(page.request, '/arca.v1.MachineService/ListMachines', {})
      const items = Array.isArray(payload.machines) ? payload.machines : []
      for (const item of items) {
        const machine = asMachineRecord(item)
        if (machine != null && machine.name === name) {
          return machine
        }
      }
      return null
    },
    (machine) => machine != null,
    options,
  ) as Promise<MachineRecord>
}

export async function waitForMachineStatus(
  page: Page,
  machineID: string,
  targetStatuses: string[],
  options: PollOptions = {},
): Promise<MachineRecord> {
  const normalizedTargets = new Set(targetStatuses.map((status) => status.trim().toLowerCase()))
  const terminalFailureStatuses = new Set(['failed', 'deleted'])

  return poll(
    async () => {
      const payload = await postJSON(page.request, '/arca.v1.MachineService/GetMachine', { machineId: machineID })
      const machine = asMachineRecord(payload.machine)
      if (machine == null) {
        throw new Error(`machine not found in response for ${machineID}`)
      }

      const status = machine.status.trim().toLowerCase()
      if (terminalFailureStatuses.has(status) && !normalizedTargets.has(status)) {
        throw new Error(`machine entered terminal status ${machine.status}`)
      }
      return machine
    },
    (machine) => normalizedTargets.has(machine.status.trim().toLowerCase()),
    options,
  )
}

export async function waitForTTYDAccess(page: Page, endpoint: string, options: PollOptions = {}): Promise<number> {
  const baseURL = new URL(`https://${endpoint}`)
  const ttydURL = new URL('/__arca/ttyd/', baseURL)

  const status = await poll(
    async () => {
      const response = await page.request.get(ttydURL.toString(), {
        failOnStatusCode: false,
        timeout: 20_000,
      })
      return response.status()
    },
    (statusCode) => [200, 401, 403].includes(statusCode),
    options,
  )

  return status
}

export async function bestEffortDeleteMachine(page: Page, machineID: string) {
  for (let attempt = 0; attempt < 5; attempt += 1) {
    const response = await page.request.post('/arca.v1.MachineService/DeleteMachine', {
      data: { machineId: machineID },
      failOnStatusCode: false,
    })

    if (response.ok()) {
      return
    }

    const payload = await parseJSONSafe(response)
    const code = String(payload?.code ?? '').toLowerCase()
    if (code.includes('not_found')) {
      return
    }

    await sleep(1500)
  }
}

export async function validateCloudflareToken(page: Page, config: CloudflareConfig) {
  const response = await page.request.post('/arca.v1.SetupService/ValidateCloudflareToken', {
    data: {
      apiToken: config.cloudflareToken,
      accountId: config.cloudflareAccountID,
    },
  })

  if (!response.ok()) {
    const payload = await parseJSONSafe(response)
    throw new Error(`ValidateCloudflareToken failed: ${response.status()} ${JSON.stringify(payload)}`)
  }

  const payload = await parseJSONSafe(response)
  const valid = payload?.valid === true
  if (!valid) {
    throw new Error(
      `ValidateCloudflareToken returned invalid for account/zone config: ${JSON.stringify(payload)}`,
    )
  }
}
