import { expect, type APIResponse, type Page } from '@playwright/test'
import { poll, sleep, type PollOptions } from './poll'

type MachineRecord = {
  id: string
  name: string
  status: string
  desiredStatus: string
  endpoint?: string
}

async function parseJSONSafe(response: APIResponse): Promise<Record<string, unknown> | null> {
  try {
    return (await response.json()) as Record<string, unknown>
  } catch {
    return null
  }
}

async function postJSON(
  page: Page,
  path: string,
  data: Record<string, unknown>,
): Promise<Record<string, unknown>> {
  const response = await page.request.post(path, { data })
  const payload = await parseJSONSafe(response)
  if (!response.ok()) {
    throw new Error(`request failed ${path}: ${response.status()} ${JSON.stringify(payload)}`)
  }
  return payload ?? {}
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

export async function createMachineViaAPI(
  page: Page,
  name: string,
  runtimeId?: string,
): Promise<string> {
  const data: Record<string, unknown> = { name }
  if (runtimeId) {
    data.runtimeId = runtimeId
  }

  const response = await page.request.post('/arca.v1.MachineService/CreateMachine', { data })
  expect(response.ok()).toBeTruthy()
  const payload = (await response.json()) as { machine?: { id?: string } }
  const machineID = payload.machine?.id?.trim() ?? ''
  expect(machineID).not.toBe('')
  return machineID
}

export async function waitForMachineByName(
  page: Page,
  name: string,
  options: PollOptions = {},
): Promise<MachineRecord> {
  return poll(
    async () => {
      const payload = await postJSON(page, '/arca.v1.MachineService/ListMachines', {})
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
      const payload = await postJSON(page, '/arca.v1.MachineService/GetMachine', {
        machineId: machineID,
      })
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

export async function waitForTTYDAccess(
  page: Page,
  endpoint: string,
  options: PollOptions = {},
): Promise<number> {
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

export async function bestEffortStopMachine(page: Page, machineID: string) {
  const response = await page.request.post('/arca.v1.MachineService/StopMachine', {
    data: { machineId: machineID },
    failOnStatusCode: false,
  })

  if (!response.ok()) {
    const payload = await parseJSONSafe(response)
    const code = String(payload?.code ?? '').toLowerCase()
    if (code.includes('not_found')) {
      return
    }
  }
}

export type { MachineRecord }
