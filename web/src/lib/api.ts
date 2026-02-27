import { create } from '@bufbuild/protobuf'
import { createClient } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'
import {
  AuthService,
  LoginRequestSchema,
  LogoutRequestSchema,
  MeRequestSchema,
  RegisterRequestSchema,
} from '@/gen/arca/v1/auth_pb'
import {
  CreateMachineRequestSchema,
  DeleteMachineRequestSchema,
  GetMachineRequestSchema,
  ListMachinesRequestSchema,
  MachineService,
  StartMachineRequestSchema,
  StopMachineRequestSchema,
  UpdateMachineRequestSchema,
} from '@/gen/arca/v1/machine_pb'
import { ApiError, parseApiErrorPayload } from '@/lib/errors'
import type { Machine, SetupStatus, User } from '@/lib/types'

const connectTransport = createConnectTransport({
  baseUrl: window.location.origin,
  fetch: (input, init) => fetch(input, { ...init, credentials: 'include' }),
})

const authClient = createClient(AuthService, connectTransport)
const machineClient = createClient(MachineService, connectTransport)

type PollingOptions = {
  timeoutMs?: number
}

async function withRequestTimeout<T>(
  timeoutMs: number | undefined,
  call: (signal?: AbortSignal) => Promise<T>,
): Promise<T> {
  if (timeoutMs == null) {
    return call()
  }

  const controller = new AbortController()
  const timer = window.setTimeout(() => controller.abort('request timeout'), timeoutMs)
  try {
    return await call(controller.signal)
  } finally {
    window.clearTimeout(timer)
  }
}

export function toUser(user: { id: string; email: string } | undefined): User | null {
  if (user == null) {
    return null
  }
  return {
    id: user.id,
    email: user.email,
  }
}

function normalizeProcedurePath(path: string): string {
  if (path.startsWith('/')) {
    return path
  }
  return `/${path}`
}

async function connectJSON<Response>(procedurePath: string, body: Record<string, unknown>): Promise<Response> {
  const response = await fetch(normalizeProcedurePath(procedurePath), {
    method: 'POST',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(body),
  })

  if (!response.ok) {
    let payload = null
    try {
      payload = parseApiErrorPayload(await response.json())
    } catch {
      payload = null
    }

    throw new ApiError(
      payload?.message ?? response.statusText ?? 'request failed',
      response.status,
      payload?.code ?? '',
    )
  }

  const contentType = response.headers.get('Content-Type') ?? ''
  if (!contentType.toLowerCase().includes('json')) {
    throw new ApiError('procedure not available', 404, 'unimplemented')
  }

  try {
    return (await response.json()) as Response
  } catch {
    throw new ApiError('invalid response', response.status, '')
  }
}

async function callConnectJSONCandidates<Response>(
  procedurePaths: string[],
  body: Record<string, unknown>,
): Promise<Response> {
  let lastError: unknown = null
  for (const path of procedurePaths) {
    try {
      return await connectJSON<Response>(path, body)
    } catch (error) {
      lastError = error
      if (error instanceof ApiError) {
        const unimplemented =
          error.status === 404 ||
          error.code.toLowerCase().includes('unimplemented') ||
          error.code.toLowerCase().includes('not_found')
        if (unimplemented) {
          continue
        }
      }
      throw error
    }
  }

  throw lastError ?? new Error('request failed')
}

export async function me(): Promise<User | null> {
  const response = await authClient.me(create(MeRequestSchema))
  return toUser(response.user)
}

export async function login(email: string, password: string): Promise<User | null> {
  const response = await authClient.login(
    create(LoginRequestSchema, {
      email,
      password,
    }),
  )
  return toUser(response.user)
}

export async function register(email: string, password: string): Promise<void> {
  await authClient.register(
    create(RegisterRequestSchema, {
      email,
      password,
    }),
  )
}

export async function logout(): Promise<void> {
  await authClient.logout(create(LogoutRequestSchema))
}

export async function listMachines(options: PollingOptions = {}): Promise<Machine[]> {
  const response = await withRequestTimeout(options.timeoutMs, (signal) =>
    machineClient.listMachines(create(ListMachinesRequestSchema), signal == null ? undefined : { signal }),
  )
  return response.machines
}

export async function createMachine(name: string): Promise<Machine> {
  const response = await machineClient.createMachine(create(CreateMachineRequestSchema, { name }))
  if (response.machine == null) {
    throw new Error('request failed')
  }
  return response.machine
}

export async function getMachine(id: string, options: PollingOptions = {}): Promise<Machine> {
  const response = await withRequestTimeout(options.timeoutMs, (signal) =>
    machineClient.getMachine(
      create(GetMachineRequestSchema, { machineId: id }),
      signal == null ? undefined : { signal },
    ),
  )
  if (response.machine == null) {
    throw new Error('request failed')
  }
  return response.machine
}

export async function updateMachine(id: string, name: string): Promise<Machine> {
  const response = await machineClient.updateMachine(
    create(UpdateMachineRequestSchema, { machineId: id, name }),
  )
  if (response.machine == null) {
    throw new Error('request failed')
  }
  return response.machine
}

export async function startMachine(id: string): Promise<Machine> {
  const response = await machineClient.startMachine(create(StartMachineRequestSchema, { machineId: id }))
  if (response.machine == null) {
    throw new Error('request failed')
  }
  return response.machine
}

export async function stopMachine(id: string): Promise<Machine> {
  const response = await machineClient.stopMachine(create(StopMachineRequestSchema, { machineId: id }))
  if (response.machine == null) {
    throw new Error('request failed')
  }
  return response.machine
}

export async function deleteMachine(id: string): Promise<void> {
  await machineClient.deleteMachine(create(DeleteMachineRequestSchema, { machineId: id }))
}

export async function getSetupStatus(): Promise<SetupStatus> {
  try {
    const response = await callConnectJSONCandidates<{
      status?: {
        completed?: boolean
        adminConfigured?: boolean
        cloudflareZoneId?: string
      }
      isConfigured?: boolean
      configured?: boolean
      setupCompleted?: boolean
      hasAdmin?: boolean
      adminConfigured?: boolean
      cloudflareZoneId?: string
    }>(
      ['/arca.v1.SetupService/GetSetupStatus', '/arca.v1.SetupService/GetStatus'],
      {},
    )

    const isConfigured =
      response.status?.completed ?? response.isConfigured ?? response.configured ?? response.setupCompleted ?? false
    const hasAdmin = response.status?.adminConfigured ?? response.hasAdmin ?? response.adminConfigured ?? false
    const cloudflareZoneID = response.status?.cloudflareZoneId ?? response.cloudflareZoneId ?? ''

    return { isConfigured, hasAdmin, cloudflareZoneID }
  } catch (error) {
    if (error instanceof ApiError && (error.status === 404 || error.code.toLowerCase().includes('unimplemented'))) {
      return { isConfigured: true, hasAdmin: true, cloudflareZoneID: '' }
    }
    throw error
  }
}

export async function setupCreateAdmin(email: string, password: string): Promise<User | null> {
  if (email.trim() === '' || password.trim() === '') {
    throw new Error('email and password are required')
  }
  return null
}

export async function setupValidateCloudflare(
  apiToken: string,
  accountID: string,
  baseDomain: string,
): Promise<void> {
  const response = await callConnectJSONCandidates<{ valid?: boolean; message?: string }>(
    ['/arca.v1.SetupService/ValidateCloudflareToken'],
    { apiToken, token: apiToken, accountId: accountID, accountID, baseDomain, domain: baseDomain },
  )
  if (response.valid !== true) {
    throw new Error(response.message ?? 'cloudflare token validation failed')
  }
}

export async function setupConfigureProviderDocker(): Promise<void> {
  return
}

export async function setupComplete(
  adminEmail: string,
  adminPassword: string,
  baseDomain: string,
  cloudflareApiToken: string,
  cloudflareZoneID: string,
): Promise<void> {
  try {
    const response = await callConnectJSONCandidates<{
      status?: {
        completed?: boolean
      }
      message?: string
    }>(['/arca.v1.SetupService/CompleteSetup'], {
      adminEmail,
      adminPassword,
      baseDomain,
      cloudflareApiToken,
      cloudflareZoneId: cloudflareZoneID,
      dockerProviderEnabled: true,
    })
    if (response.status?.completed !== true) {
      throw new Error(response.message ?? 'setup completion failed')
    }
  } catch (error) {
    if (error instanceof ApiError && (error.status === 404 || error.code.toLowerCase().includes('unimplemented'))) {
      return
    }
    throw error
  }
}
