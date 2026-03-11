import { create } from '@bufbuild/protobuf'
import { createClient } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'
import {
  AuthService,
  CompleteOidcLoginRequestSchema,
  LoginRequestSchema,
  LogoutRequestSchema,
  MeRequestSchema,
  StartOidcLoginRequestSchema,
} from '@/gen/arca/v1/auth_pb'
import {
  CreateMachineRequestSchema,
  DeleteMachineRequestSchema,
  GetMachineRequestSchema,
  ListMachinesRequestSchema,
  ListMachineEventsRequestSchema,
  MachineService,
  StartMachineRequestSchema,
  StopMachineRequestSchema,
  UpdateMachineRequestSchema,
} from '@/gen/arca/v1/machine_pb'
import {
  CreateRuntimeRequestSchema,
  DeleteRuntimeRequestSchema,
  ListAvailableRuntimesRequestSchema,
  ListRuntimesRequestSchema,
  RuntimeService,
  RuntimeType,
  UpdateRuntimeRequestSchema,
} from '@/gen/arca/v1/runtime_pb'
import {
  ListMachineExposuresRequestSchema,
  TunnelService,
} from '@/gen/arca/v1/tunnel_pb'
import {
  GetMachineSharingRequestSchema,
  SharingService,
  UpdateMachineSharingRequestSchema,
} from '@/gen/arca/v1/sharing_pb'
import type { GeneralAccess, MachineSharingMember } from '@/gen/arca/v1/sharing_pb'
import {
  CompleteUserSetupRequestSchema,
  CreateUserRequestSchema,
  GetUserSettingsRequestSchema,
  IssueUserSetupTokenRequestSchema,
  ListUsersRequestSchema as ListManagedUsersRequestSchema,
  UpdateUserSettingsRequestSchema,
  UpdateUserRoleRequestSchema,
  UserService,
} from '@/gen/arca/v1/user_pb'
import { ApiError, parseApiErrorPayload } from '@/lib/errors'
import type {
  Machine,
  MachineEvent,
  MachineExposure,
  MachineExposureConfig,
  MachineExposureMethodType,
  ManagedUser,
  RuntimeCatalogConfig,
  RuntimeCatalogItem,
  RuntimeCatalogType,
  RuntimeSummary,
  ServerExposureMethod,
  SetupStatus,
  User,
  UserSettings,
} from '@/lib/types'

const connectTransport = createConnectTransport({
  baseUrl: window.location.origin,
  fetch: (input, init) => fetch(input, { ...init, credentials: 'include' }),
})

const authClient = createClient(AuthService, connectTransport)
const machineClient = createClient(MachineService, connectTransport)
const runtimeClient = createClient(RuntimeService, connectTransport)
const tunnelClient = createClient(TunnelService, connectTransport)
const userClient = createClient(UserService, connectTransport)
const sharingClient = createClient(SharingService, connectTransport)

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

export function toUser(user: { id: string; email: string; role?: string } | undefined): User | null {
  if (user == null) {
    return null
  }
  return {
    id: user.id,
    email: user.email,
    role: user.role ?? 'user',
  }
}

function toManagedUser(user: {
  id: string
  email: string
  setupRequired: boolean
  role?: string
  setupTokenExpiresAt: bigint
  createdAt: bigint
} | undefined): ManagedUser | null {
  if (user == null) {
    return null
  }
  return {
    id: user.id,
    email: user.email,
    setupRequired: user.setupRequired,
    role: user.role ?? 'user',
    setupTokenExpiresAt: Number(user.setupTokenExpiresAt),
    createdAt: Number(user.createdAt),
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

export async function startOidcLogin(redirectURI: string): Promise<string> {
  const response = await authClient.startOidcLogin(
    create(StartOidcLoginRequestSchema, {
      redirectUri: redirectURI,
    }),
  )
  return response.authorizationUrl
}

export async function completeOidcLogin(code: string, state: string, redirectURI: string): Promise<User | null> {
  const response = await authClient.completeOidcLogin(
    create(CompleteOidcLoginRequestSchema, {
      code,
      state,
      redirectUri: redirectURI,
    }),
  )
  return toUser(response.user)
}

export async function logout(): Promise<void> {
  await authClient.logout(create(LogoutRequestSchema))
}

export async function listManagedUsers(): Promise<ManagedUser[]> {
  const response = await userClient.listUsers(create(ListManagedUsersRequestSchema))
  return response.users
    .map((user) => toManagedUser(user))
    .filter((user): user is ManagedUser => user != null)
}

export async function createManagedUser(email: string): Promise<{ user: ManagedUser; setupToken: string; setupTokenExpiresAt: number }> {
  const response = await userClient.createUser(create(CreateUserRequestSchema, { email }))
  const user = toManagedUser(response.user)
  if (user == null) {
    throw new Error('request failed')
  }
  return {
    user,
    setupToken: response.setupToken,
    setupTokenExpiresAt: Number(response.setupTokenExpiresAt),
  }
}

export async function issueManagedUserSetupToken(userID: string): Promise<{ user: ManagedUser; setupToken: string; setupTokenExpiresAt: number }> {
  const response = await userClient.issueUserSetupToken(create(IssueUserSetupTokenRequestSchema, { userId: userID }))
  const user = toManagedUser(response.user)
  if (user == null) {
    throw new Error('request failed')
  }
  return {
    user,
    setupToken: response.setupToken,
    setupTokenExpiresAt: Number(response.setupTokenExpiresAt),
  }
}

export async function updateUserRole(userID: string, role: string): Promise<ManagedUser> {
  const response = await userClient.updateUserRole(create(UpdateUserRoleRequestSchema, { userId: userID, role }))
  const user = toManagedUser(response.user)
  if (user == null) {
    throw new Error('request failed')
  }
  return user
}

export async function completeUserSetup(setupToken: string, password: string): Promise<User | null> {
  const response = await userClient.completeUserSetup(
    create(CompleteUserSetupRequestSchema, {
      setupToken,
      password,
    }),
  )
  return toUser(response.user)
}

export async function getUserSettings(): Promise<UserSettings> {
  const response = await userClient.getUserSettings(create(GetUserSettingsRequestSchema))
  return {
    sshPublicKeys: response.settings?.sshPublicKeys ?? [],
  }
}

export async function updateUserSettings(sshPublicKeys: string[]): Promise<UserSettings> {
  const response = await userClient.updateUserSettings(
    create(UpdateUserSettingsRequestSchema, {
      settings: {
        sshPublicKeys,
      },
    }),
  )
  return {
    sshPublicKeys: response.settings?.sshPublicKeys ?? [],
  }
}

export async function listMachines(options: PollingOptions = {}): Promise<Machine[]> {
  const response = await withRequestTimeout(options.timeoutMs, (signal) =>
    machineClient.listMachines(create(ListMachinesRequestSchema), signal == null ? undefined : { signal }),
  )
  return response.machines
}

export async function createMachine(name: string, runtimeID: string): Promise<Machine> {
  const response = await machineClient.createMachine(create(CreateMachineRequestSchema, { name, runtimeId: runtimeID })) 
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

export async function startMachine(id: string, runtimeID?: string): Promise<Machine> {
  const response = await machineClient.startMachine(create(StartMachineRequestSchema, { machineId: id, runtimeId: runtimeID ?? '' }))
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

export async function listMachineEvents(id: string, limit = 100, options: PollingOptions = {}): Promise<MachineEvent[]> {
  const response = await withRequestTimeout(options.timeoutMs, (signal) =>
    machineClient.listMachineEvents(
      create(ListMachineEventsRequestSchema, { machineId: id, limit }),
      signal == null ? undefined : { signal },
    ),
  )
  return response.events
}

export async function deleteMachine(id: string): Promise<void> {
  await machineClient.deleteMachine(create(DeleteMachineRequestSchema, { machineId: id }))
}

function runtimeTypeToProto(type: RuntimeCatalogType): RuntimeType {
  if (type === 'gce') return RuntimeType.GCE
  if (type === 'lxd') return RuntimeType.LXD
  return RuntimeType.LIBVIRT
}

function runtimeTypeFromProto(type: RuntimeType): RuntimeCatalogType {
  if (type === RuntimeType.GCE) return 'gce'
  if (type === RuntimeType.LXD) return 'lxd'
  return 'libvirt'
}

function machineExposureMethodFromProto(method: number): MachineExposureMethodType {
  return method === 2 ? 'proxy_via_server' : 'cloudflare_tunnel'
}

function machineExposureMethodToProto(method: MachineExposureMethodType): number {
  return method === 'proxy_via_server' ? 2 : 1
}

function toRuntimeCatalogItem(input: {
  id: string
  name: string
  type: RuntimeType
  config?: {
    provider:
      | { case: 'libvirt'; value: { uri: string; network: string; storagePool: string; startupScript: string } }
      | {
          case: 'gce'
          value: { project: string; zone: string; network: string; subnetwork: string; serviceAccountEmail: string; startupScript: string; machineType: string; diskSizeGb: bigint; imageProject: string; imageFamily: string }
        }
      | { case: 'lxd'; value: { endpoint: string; startupScript: string } }
      | { case: undefined; value?: undefined }
    exposure?: {
      method?: number
      domainPrefix?: string
      baseDomain?: string
      cloudflareApiToken?: string
      cloudflareAccountId?: string
      cloudflareZoneId?: string
      connectivity?: number
    }
    serverApiUrl?: string
  }
  createdAt: bigint
  updatedAt: bigint
}): RuntimeCatalogItem {
  const runtimeType = runtimeTypeFromProto(input.type)
  let config: RuntimeCatalogConfig
  if (runtimeType === 'gce') {
    const gce = input.config?.provider.case === 'gce' ? input.config.provider.value : undefined
    config = {
      type: 'gce',
      project: gce?.project ?? '',
      zone: gce?.zone ?? '',
      network: gce?.network ?? '',
      subnetwork: gce?.subnetwork ?? '',
      serviceAccountEmail: gce?.serviceAccountEmail ?? '',
      startupScript: gce?.startupScript ?? '',
      machineType: gce?.machineType ?? '',
      diskSizeGb: Number(gce?.diskSizeGb ?? 0),
      imageProject: gce?.imageProject ?? '',
      imageFamily: gce?.imageFamily ?? '',
    }
  } else if (runtimeType === 'lxd') {
    const lxd = input.config?.provider.case === 'lxd' ? input.config.provider.value : undefined
    config = {
      type: 'lxd',
      endpoint: lxd?.endpoint ?? '',
      startupScript: lxd?.startupScript ?? '',
    }
  } else {
    const libvirt = input.config?.provider.case === 'libvirt' ? input.config.provider.value : undefined
    config = {
      type: 'libvirt',
      uri: libvirt?.uri ?? '',
      network: libvirt?.network ?? '',
      storagePool: libvirt?.storagePool ?? '',
      startupScript: libvirt?.startupScript ?? '',
    }
  }

  const exposureInput = input.config?.exposure
  const connectivityNum = exposureInput?.connectivity ?? 0
  const exposure: MachineExposureConfig = {
    method: machineExposureMethodFromProto(exposureInput?.method ?? 0),
    domainPrefix: exposureInput?.domainPrefix ?? '',
    baseDomain: exposureInput?.baseDomain ?? '',
    cloudflareApiToken: exposureInput?.cloudflareApiToken ?? '',
    cloudflareAccountId: exposureInput?.cloudflareAccountId ?? '',
    cloudflareZoneId: exposureInput?.cloudflareZoneId ?? '',
    connectivity: connectivityNum === 1 ? 'private_ip' : connectivityNum === 2 ? 'public_ip' : '',
  }

  return {
    id: input.id,
    name: input.name,
    type: runtimeType,
    config,
    exposure,
    serverApiUrl: input.config?.serverApiUrl ?? '',
    createdAt: Number(input.createdAt),
    updatedAt: Number(input.updatedAt),
  }
}

function runtimeConfigPayload(type: RuntimeCatalogType, config: RuntimeCatalogConfig, exposure?: MachineExposureConfig, serverApiUrl?: string) {
  let provider
  if (type === 'gce') {
    if (config.type !== 'gce') {
      throw new Error('gce config is required')
    }
    provider = {
      case: 'gce' as const,
      value: {
        project: config.project,
        zone: config.zone,
        network: config.network,
        subnetwork: config.subnetwork,
        serviceAccountEmail: config.serviceAccountEmail,
        startupScript: config.startupScript,
        machineType: config.machineType,
        diskSizeGb: BigInt(config.diskSizeGb || 0),
        imageProject: config.imageProject,
        imageFamily: config.imageFamily,
      },
    }
  } else if (type === 'lxd') {
    if (config.type !== 'lxd') {
      throw new Error('lxd config is required')
    }
    provider = {
      case: 'lxd' as const,
      value: {
        endpoint: config.endpoint,
        startupScript: config.startupScript,
      },
    }
  } else {
    if (config.type !== 'libvirt') {
      throw new Error('libvirt config is required')
    }
    provider = {
      case: 'libvirt' as const,
      value: {
        uri: config.uri,
        network: config.network,
        storagePool: config.storagePool,
        startupScript: config.startupScript,
      },
    }
  }

  const result: Record<string, unknown> = { provider }
  if (exposure) {
    result.exposure = {
      method: machineExposureMethodToProto(exposure.method),
      domainPrefix: exposure.domainPrefix,
      baseDomain: exposure.baseDomain,
      cloudflareApiToken: exposure.cloudflareApiToken,
      cloudflareAccountId: exposure.cloudflareAccountId,
      cloudflareZoneId: exposure.cloudflareZoneId,
      connectivity: exposure.connectivity === 'private_ip' ? 1 : exposure.connectivity === 'public_ip' ? 2 : 0,
    }
  }
  if (serverApiUrl) {
    result.serverApiUrl = serverApiUrl
  }
  return result
}

export async function listRuntimes(): Promise<RuntimeCatalogItem[]> {
  const response = await runtimeClient.listRuntimes(create(ListRuntimesRequestSchema))
  return response.runtimes.map((runtime) => toRuntimeCatalogItem(runtime))
}

export async function listAvailableRuntimes(): Promise<RuntimeSummary[]> {
  const response = await runtimeClient.listAvailableRuntimes(create(ListAvailableRuntimesRequestSchema))
  return response.runtimes.map((runtime) => ({
    id: runtime.id,
    name: runtime.name,
    type: runtimeTypeFromProto(runtime.type),
  }))
}

export async function createRuntime(
  name: string,
  type: RuntimeCatalogType,
  config: RuntimeCatalogConfig,
  exposure?: MachineExposureConfig,
  serverApiUrl?: string,
): Promise<RuntimeCatalogItem> {
  const response = await runtimeClient.createRuntime(
    create(CreateRuntimeRequestSchema, {
      name,
      type: runtimeTypeToProto(type),
      config: runtimeConfigPayload(type, config, exposure, serverApiUrl),
    }),
  )
  if (response.runtime == null) {
    throw new Error('request failed')
  }
  return toRuntimeCatalogItem(response.runtime)
}

export async function updateRuntime(
  runtimeID: string,
  name: string,
  type: RuntimeCatalogType,
  config: RuntimeCatalogConfig,
  exposure?: MachineExposureConfig,
  serverApiUrl?: string,
): Promise<RuntimeCatalogItem> {
  const response = await runtimeClient.updateRuntime(
    create(UpdateRuntimeRequestSchema, {
      runtimeId: runtimeID,
      name,
      type: runtimeTypeToProto(type),
      config: runtimeConfigPayload(type, config, exposure, serverApiUrl),
    }),
  )
  if (response.runtime == null) {
    throw new Error('request failed')
  }
  return toRuntimeCatalogItem(response.runtime)
}

export async function deleteRuntime(runtimeID: string): Promise<void> {
  await runtimeClient.deleteRuntime(create(DeleteRuntimeRequestSchema, { runtimeId: runtimeID }))
}

export async function getSetupStatus(): Promise<SetupStatus> {
  try {
    const response = await callConnectJSONCandidates<{
      status?: {
        completed?: boolean
        adminConfigured?: boolean
        cloudflareZoneId?: string
        machineRuntime?: string
        internetPublicExposureDisabled?: boolean
        oidcEnabled?: boolean
        oidcIssuerUrl?: string
        oidcClientId?: string
        oidcClientSecretConfigured?: boolean
        oidcAllowedEmailDomains?: string[]
        serverExposureMethod?: number | string
        serverDomain?: string
        passwordLoginDisabled?: boolean
        iapEnabled?: boolean
        iapAudience?: string
        iapAutoProvisioning?: boolean
        oidcAutoProvisioning?: boolean
      }
      isConfigured?: boolean
      configured?: boolean
      setupCompleted?: boolean
      hasAdmin?: boolean
      adminConfigured?: boolean
      cloudflareZoneId?: string
      machineRuntime?: string
      internetPublicExposureDisabled?: boolean
      oidcEnabled?: boolean
      oidcIssuerUrl?: string
      oidcClientId?: string
      oidcClientSecretConfigured?: boolean
      oidcAllowedEmailDomains?: string[]
      serverExposureMethod?: number | string
      serverDomain?: string
      passwordLoginDisabled?: boolean
      iapEnabled?: boolean
      iapAudience?: string
      iapAutoProvisioning?: boolean
      oidcAutoProvisioning?: boolean
    }>(
      ['/arca.v1.SetupService/GetSetupStatus', '/arca.v1.SetupService/GetStatus'],
      {},
    )

    const isConfigured =
      response.status?.completed ?? response.isConfigured ?? response.configured ?? response.setupCompleted ?? false
    const hasAdmin = response.status?.adminConfigured ?? response.hasAdmin ?? response.adminConfigured ?? false
    const cloudflareZoneID = response.status?.cloudflareZoneId ?? response.cloudflareZoneId ?? ''
    const internetPublicExposureDisabled =
      response.status?.internetPublicExposureDisabled ?? response.internetPublicExposureDisabled ?? false
    const oidcEnabled = response.status?.oidcEnabled ?? response.oidcEnabled ?? false
    const oidcIssuerURL = response.status?.oidcIssuerUrl ?? response.oidcIssuerUrl ?? ''
    const oidcClientID = response.status?.oidcClientId ?? response.oidcClientId ?? ''
    const oidcClientSecretConfigured =
      response.status?.oidcClientSecretConfigured ?? response.oidcClientSecretConfigured ?? false
    const oidcAllowedEmailDomains =
      response.status?.oidcAllowedEmailDomains ?? response.oidcAllowedEmailDomains ?? []
    const serverExposureMethodRaw =
      response.status?.serverExposureMethod ?? response.serverExposureMethod ?? 0
    const serverExposureMethod: ServerExposureMethod =
      serverExposureMethodRaw === 2 || serverExposureMethodRaw === 'SERVER_EXPOSURE_METHOD_MANUAL'
        ? 'manual'
        : 'cloudflare_tunnel'
    const serverDomain = response.status?.serverDomain ?? response.serverDomain ?? ''
    const passwordLoginDisabled =
      response.status?.passwordLoginDisabled ?? response.passwordLoginDisabled ?? false
    const iapEnabled = response.status?.iapEnabled ?? response.iapEnabled ?? false
    const iapAudience = response.status?.iapAudience ?? response.iapAudience ?? ''
    const iapAutoProvisioning = response.status?.iapAutoProvisioning ?? response.iapAutoProvisioning ?? false
    const oidcAutoProvisioning = response.status?.oidcAutoProvisioning ?? response.oidcAutoProvisioning ?? false

    return {
      isConfigured,
      hasAdmin,
      cloudflareZoneID,
      baseDomain: '',
      domainPrefix: '',
      internetPublicExposureDisabled,
      oidcEnabled,
      oidcIssuerURL,
      oidcClientID,
      oidcClientSecretConfigured,
      oidcAllowedEmailDomains,
      passwordLoginDisabled,
      iapEnabled,
      iapAudience,
      iapAutoProvisioning,
      oidcAutoProvisioning,
      serverExposureMethod,
      serverDomain,
    }
  } catch (error) {
    if (error instanceof ApiError && (error.status === 404 || error.code.toLowerCase().includes('unimplemented'))) {
      return {
        isConfigured: true,
        hasAdmin: true,
        cloudflareZoneID: '',
        baseDomain: '',
        domainPrefix: '',
        internetPublicExposureDisabled: false,
        oidcEnabled: false,
        oidcIssuerURL: '',
        oidcClientID: '',
        oidcClientSecretConfigured: false,
        oidcAllowedEmailDomains: [],
        passwordLoginDisabled: false,
        iapEnabled: false,
        iapAudience: '',
        iapAutoProvisioning: false,
        oidcAutoProvisioning: false,
        serverExposureMethod: 'cloudflare_tunnel',
        serverDomain: '',
      }
    }
    throw error
  }
}

export async function verifySetupPassword(password: string): Promise<boolean> {
  const response = await connectJSON<{ valid?: boolean }>(
    '/arca.v1.SetupService/VerifySetupPassword',
    { setupPassword: password },
  )
  return response.valid === true
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

export async function setupComplete(
  adminEmail: string,
  adminPassword: string,
  cloudflareApiToken: string,
  cloudflareZoneID: string,
  serverExposureMethod: ServerExposureMethod = 'cloudflare_tunnel',
  serverDomain: string = '',
  setupPassword: string = '',
): Promise<void> {
  try {
    const serverExposureMethodNum = serverExposureMethod === 'manual' ? 2 : 1
    const response = await callConnectJSONCandidates<{
      status?: {
        completed?: boolean
      }
      message?: string
    }>(['/arca.v1.SetupService/CompleteSetup'], {
      adminEmail,
      adminPassword,
      cloudflareApiToken,
      cloudflareZoneId: cloudflareZoneID,
      serverExposureMethod: serverExposureMethodNum,
      serverDomain,
      setupPassword,
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

export async function updateDomainSettings(
  disableInternetPublicExposure: boolean,
  oidcEnabled: boolean,
  oidcIssuerURL: string,
  oidcClientID: string,
  oidcClientSecret: string,
  oidcAllowedEmailDomains: string[],
  clearOidcClientSecret: boolean,
  serverExposureMethod: ServerExposureMethod = 'cloudflare_tunnel',
  serverDomain: string = '',
  cloudflareApiToken: string = '',
  cloudflareZoneID: string = '',
  passwordLoginDisabled: boolean = false,
  iapEnabled: boolean = false,
  iapAudience: string = '',
  iapAutoProvisioning: boolean = false,
  oidcAutoProvisioning: boolean = false,
): Promise<void> {
  const serverExposureMethodNum = serverExposureMethod === 'manual' ? 2 : 1
  const response = await callConnectJSONCandidates<{
    status?: {
      completed?: boolean
      serverDomain?: string
    }
    message?: string
  }>(['/arca.v1.SetupService/UpdateDomainSettings'], {
    disableInternetPublicExposure,
    oidcEnabled,
    oidcIssuerUrl: oidcIssuerURL,
    oidcClientId: oidcClientID,
    oidcClientSecret,
    oidcAllowedEmailDomains,
    clearOidcClientSecret,
    serverExposureMethod: serverExposureMethodNum,
    serverDomain,
    cloudflareApiToken,
    cloudflareZoneId: cloudflareZoneID,
    passwordLoginDisabled,
    iapEnabled,
    iapAudience,
    iapAutoProvisioning,
    oidcAutoProvisioning,
  })
  if (response.status?.completed !== true) {
    throw new Error(response.message ?? 'failed to update domain settings')
  }
}

export async function listMachineExposures(machineID: string): Promise<MachineExposure[]> {
  const response = await tunnelClient.listMachineExposures(
    create(ListMachineExposuresRequestSchema, { machineId: machineID }),
  )
  return response.exposures
}

export async function getMachineSharing(machineID: string): Promise<{
  members: MachineSharingMember[]
  generalAccess: GeneralAccess | undefined
}> {
  const response = await sharingClient.getMachineSharing(
    create(GetMachineSharingRequestSchema, { machineId: machineID }),
  )
  return {
    members: response.members,
    generalAccess: response.generalAccess,
  }
}

export async function updateMachineSharing(
  machineID: string,
  members: { userId: string; email: string; role: string }[],
  generalAccess: { scope: string; role: string },
): Promise<{
  members: MachineSharingMember[]
  generalAccess: GeneralAccess | undefined
}> {
  const response = await sharingClient.updateMachineSharing(
    create(UpdateMachineSharingRequestSchema, {
      machineId: machineID,
      members,
      generalAccess,
    }),
  )
  return {
    members: response.members,
    generalAccess: response.generalAccess,
  }
}
