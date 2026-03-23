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
  ChangeMachineProfileRequestSchema,
  CreateMachineRequestSchema,
  DeleteMachineRequestSchema,
  GetMachineRequestSchema,
  ListMachinesRequestSchema,
  ListMachineEventsRequestSchema,
  MachineService,
  StartMachineRequestSchema,
  StopMachineRequestSchema,
  RestartMachineRequestSchema,
  UpdateMachineRequestSchema,
  UpdateMachineTagsRequestSchema,
  CreateImageFromMachineRequestSchema,
} from '@/gen/arca/v1/machine_pb'
import {
  CreateMachineProfileRequestSchema,
  DeleteMachineProfileRequestSchema,
  ListAvailableProfilesRequestSchema,
  ListMachineProfilesRequestSchema,
  MachineProfileService,
  MachineProfileType,
  UpdateMachineProfileRequestSchema,
} from '@/gen/arca/v1/machine_profile_pb'
import {
  ListMachineExposuresRequestSchema,
  ExposureService,
} from '@/gen/arca/v1/exposure_pb'
import {
  GetMachineSharingRequestSchema,
  ListMachineAccessRequestsRequestSchema,
  RequestMachineAccessRequestSchema,
  ResolveMachineAccessRequestRequestSchema,
  SharingService,
  UpdateMachineSharingRequestSchema,
} from '@/gen/arca/v1/sharing_pb'
import {
  AddGroupMemberRequestSchema,
  CreateGroupRequestSchema,
  DeleteGroupRequestSchema,
  GetGroupRequestSchema,
  GroupService,
  ListGroupsRequestSchema,
  RemoveGroupMemberRequestSchema,
  SearchGroupsRequestSchema,
} from '@/gen/arca/v1/group_pb'
import type { UserGroup, UserGroupMember } from '@/gen/arca/v1/group_pb'
import {
  GetSlackConfigRequestSchema,
  GetUserNotificationSettingsRequestSchema,
  NotificationService,
  TestSlackNotificationRequestSchema,
  UpdateSlackConfigRequestSchema,
  UpdateUserNotificationSettingsRequestSchema,
} from '@/gen/arca/v1/notification_pb'
import type { GeneralAccess, MachineAccessRequest, MachineSharingGroup, MachineSharingMember } from '@/gen/arca/v1/sharing_pb'
import {
  CompleteUserSetupRequestSchema,
  CreateUserRequestSchema,
  CreateUserLLMModelRequestSchema,
  DeleteUserLLMModelRequestSchema,
  DuplicateUserLLMModelRequestSchema,
  GetUserAgentPromptRequestSchema,
  GetUserStartupScriptRequestSchema,
  IssueUserSetupTokenRequestSchema,
  ListUserLLMModelsRequestSchema,
  ListUsersRequestSchema as ListManagedUsersRequestSchema,
  SearchUsersRequestSchema,
  UpdateUserAgentPromptRequestSchema,
  UpdateUserLLMModelRequestSchema,
  UpdateUserRoleRequestSchema,
  UpdateUserStartupScriptRequestSchema,
  UserService,
} from '@/gen/arca/v1/user_pb'
import type { LLMModel } from '@/gen/arca/v1/user_pb'
import { ApiError, parseApiErrorPayload } from '@/lib/errors'
import type {
  Machine,
  MachineEvent,
  MachineExposure,
  MachineExposureConfig,
  MachineExposureMethodType,
  ManagedUser,
  MachineProfileConfig,
  MachineProfileItem,
  MachineProfileType as MachineProfileTypeLocal,
  MachineProfileSummary,
  SetupStatus,
  User,
} from '@/lib/types'

const connectTransport = createConnectTransport({
  baseUrl: window.location.origin,
  fetch: (input, init) => fetch(input, { ...init, credentials: 'include' }),
})

const authClient = createClient(AuthService, connectTransport)
const machineClient = createClient(MachineService, connectTransport)
const profileClient = createClient(MachineProfileService, connectTransport)
const exposureClient = createClient(ExposureService, connectTransport)
const userClient = createClient(UserService, connectTransport)
const sharingClient = createClient(SharingService, connectTransport)
const groupClient = createClient(GroupService, connectTransport)
const notificationClient = createClient(NotificationService, connectTransport)

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

export async function listMachines(options: PollingOptions = {}): Promise<Machine[]> {
  const response = await withRequestTimeout(options.timeoutMs, (signal) =>
    machineClient.listMachines(create(ListMachinesRequestSchema), signal == null ? undefined : { signal }),
  )
  return response.machines
}

export async function createMachine(name: string, profileID: string, options?: Record<string, string>, customImageId?: string, tags?: string[]): Promise<Machine> {
  const response = await machineClient.createMachine(create(CreateMachineRequestSchema, { name, profileId: profileID, options: options ?? {}, customImageId: customImageId ?? '', tags: tags ?? [] }))
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

export async function updateMachine(id: string, name: string, options?: Record<string, string>): Promise<Machine> {
  const response = await machineClient.updateMachine(
    create(UpdateMachineRequestSchema, { machineId: id, name, options: options ?? {} }),
  )
  if (response.machine == null) {
    throw new Error('request failed')
  }
  return response.machine
}

export async function updateMachineOptions(id: string, options: Record<string, string>): Promise<Machine> {
  return updateMachine(id, '', options)
}

export async function updateMachineTags(id: string, tags: string[]): Promise<Machine> {
  const response = await machineClient.updateMachineTags(
    create(UpdateMachineTagsRequestSchema, { machineId: id, tags }),
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

export async function restartMachine(id: string): Promise<Machine> {
  const response = await machineClient.restartMachine(create(RestartMachineRequestSchema, { machineId: id }))
  if (response.machine == null) {
    throw new Error('request failed')
  }
  return response.machine
}

export async function createImageFromMachine(machineId: string, name: string, description: string): Promise<string> {
  const response = await machineClient.createImageFromMachine(
    create(CreateImageFromMachineRequestSchema, { machineId, name, description })
  )
  return response.jobId
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

function profileTypeToProto(type: MachineProfileTypeLocal): MachineProfileType {
  if (type === 'gce') return MachineProfileType.GCE
  if (type === 'lxd') return MachineProfileType.LXD
  if (type === 'mock') return MachineProfileType.MOCK
  return MachineProfileType.LIBVIRT
}

function profileTypeFromProto(type: MachineProfileType): MachineProfileTypeLocal {
  if (type === MachineProfileType.GCE) return 'gce'
  if (type === MachineProfileType.LXD) return 'lxd'
  if (type === MachineProfileType.MOCK) return 'mock'
  return 'libvirt'
}

function machineExposureMethodFromProto(_method: number): MachineExposureMethodType {
  return 'proxy_via_server'
}

function machineExposureMethodToProto(_method: MachineExposureMethodType): number {
  return 2
}

function toMachineProfileItem(input: {
  id: string
  name: string
  type: MachineProfileType
  config?: {
    provider:
      | { case: 'libvirt'; value: { uri: string; network: string; storagePool: string; startupScript: string } }
      | {
          case: 'gce'
          value: { project: string; zone: string; network: string; subnetwork: string; serviceAccountEmail: string; startupScript: string; diskSizeGb: bigint; allowedMachineTypes: string[] }
        }
      | { case: 'lxd'; value: { endpoint: string; startupScript: string } }
      | { case: 'mock'; value: object }
      | { case: undefined; value?: undefined }
    exposure?: {
      method?: number
      connectivity?: number
    }
    serverApiUrl?: string
    autoStopTimeoutSeconds?: bigint
    agentPrompt?: string
  }
  createdAt: bigint
  updatedAt: bigint
  machineCount?: number
  runningMachineCount?: number
}): MachineProfileItem {
  const profileType = profileTypeFromProto(input.type)
  let config: MachineProfileConfig
  if (profileType === 'gce') {
    const gce = input.config?.provider.case === 'gce' ? input.config.provider.value : undefined
    config = {
      type: 'gce',
      project: gce?.project ?? '',
      zone: gce?.zone ?? '',
      network: gce?.network ?? '',
      subnetwork: gce?.subnetwork ?? '',
      serviceAccountEmail: gce?.serviceAccountEmail ?? '',
      startupScript: gce?.startupScript ?? '',
      diskSizeGb: Number(gce?.diskSizeGb ?? 0),
      allowedMachineTypes: gce?.allowedMachineTypes ?? [],
    }
  } else if (profileType === 'lxd') {
    const lxd = input.config?.provider.case === 'lxd' ? input.config.provider.value : undefined
    config = {
      type: 'lxd',
      endpoint: lxd?.endpoint ?? '',
      startupScript: lxd?.startupScript ?? '',
    }
  } else if (profileType === 'mock') {
    config = { type: 'mock' }
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
    connectivity: connectivityNum === 1 ? 'private_ip' : connectivityNum === 2 ? 'public_ip' : '',
  }

  return {
    id: input.id,
    name: input.name,
    type: profileType,
    config,
    exposure,
    serverApiUrl: input.config?.serverApiUrl ?? '',
    autoStopTimeoutSeconds: Number(input.config?.autoStopTimeoutSeconds ?? 0),
    agentPrompt: input.config?.agentPrompt ?? '',
    createdAt: Number(input.createdAt),
    updatedAt: Number(input.updatedAt),
    machineCount: input.machineCount ?? 0,
    runningMachineCount: input.runningMachineCount ?? 0,
  }
}

function profileConfigPayload(type: MachineProfileTypeLocal, config: MachineProfileConfig, exposure?: MachineExposureConfig, serverApiUrl?: string, autoStopTimeoutSeconds?: number, agentPrompt?: string) {
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
        diskSizeGb: BigInt(config.diskSizeGb || 0),
        allowedMachineTypes: config.allowedMachineTypes ?? [],
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
  } else if (type === 'mock') {
    provider = {
      case: 'mock' as const,
      value: {},
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
      connectivity: exposure.connectivity === 'private_ip' ? 1 : exposure.connectivity === 'public_ip' ? 2 : 0,
    }
  }
  if (serverApiUrl) {
    result.serverApiUrl = serverApiUrl
  }
  if (autoStopTimeoutSeconds != null && autoStopTimeoutSeconds > 0) {
    result.autoStopTimeoutSeconds = BigInt(autoStopTimeoutSeconds)
  }
  if (agentPrompt != null) {
    result.agentPrompt = agentPrompt
  }
  return result
}

export async function listMachineProfiles(): Promise<MachineProfileItem[]> {
  const response = await profileClient.listMachineProfiles(create(ListMachineProfilesRequestSchema))
  return response.profiles.map((profile) => toMachineProfileItem(profile))
}

export async function listAvailableProfiles(): Promise<MachineProfileSummary[]> {
  const response = await profileClient.listAvailableProfiles(create(ListAvailableProfilesRequestSchema))
  return response.profiles.map((profile) => ({
    id: profile.id,
    name: profile.name,
    type: profileTypeFromProto(profile.type),
    allowedMachineTypes: profile.allowedMachineTypes,
  }))
}

export async function createMachineProfile(
  name: string,
  type: MachineProfileTypeLocal,
  config: MachineProfileConfig,
  exposure?: MachineExposureConfig,
  serverApiUrl?: string,
  autoStopTimeoutSeconds?: number,
  agentPrompt?: string,
): Promise<MachineProfileItem> {
  const response = await profileClient.createMachineProfile(
    create(CreateMachineProfileRequestSchema, {
      name,
      type: profileTypeToProto(type),
      config: profileConfigPayload(type, config, exposure, serverApiUrl, autoStopTimeoutSeconds, agentPrompt),
    }),
  )
  if (response.profile == null) {
    throw new Error('request failed')
  }
  return toMachineProfileItem(response.profile)
}

// Backward-compatible aliases
export const listMachineTemplates = listMachineProfiles
export const listAvailableMachineTemplates = listAvailableProfiles

export async function updateMachineProfile(
  profileID: string,
  name: string,
  type: MachineProfileTypeLocal,
  config: MachineProfileConfig,
  exposure?: MachineExposureConfig,
  serverApiUrl?: string,
  autoStopTimeoutSeconds?: number,
  agentPrompt?: string,
): Promise<MachineProfileItem> {
  const response = await profileClient.updateMachineProfile(
    create(UpdateMachineProfileRequestSchema, {
      profileId: profileID,
      name,
      type: profileTypeToProto(type),
      config: profileConfigPayload(type, config, exposure, serverApiUrl, autoStopTimeoutSeconds, agentPrompt),
    }),
  )
  if (response.profile == null) {
    throw new Error('request failed')
  }
  return toMachineProfileItem(response.profile)
}

export async function deleteMachineProfile(profileID: string): Promise<void> {
  await profileClient.deleteMachineProfile(create(DeleteMachineProfileRequestSchema, { profileId: profileID }))
}

export async function changeMachineProfile(machineID: string, profileID: string): Promise<Machine> {
  const response = await machineClient.changeMachineProfile(
    create(ChangeMachineProfileRequestSchema, { machineId: machineID, profileId: profileID }),
  )
  if (response.machine == null) {
    throw new Error('request failed')
  }
  return response.machine
}

export async function getSetupStatus(): Promise<SetupStatus> {
  try {
    const response = await callConnectJSONCandidates<{
      status?: {
        completed?: boolean
        adminConfigured?: boolean
        machineRuntime?: string
        internetPublicExposureDisabled?: boolean
        oidcEnabled?: boolean
        oidcIssuerUrl?: string
        oidcClientId?: string
        oidcClientSecretConfigured?: boolean
        oidcAllowedEmailDomains?: string[]
        serverDomain?: string
        baseDomain?: string
        domainPrefix?: string
        passwordLoginDisabled?: boolean
        iapEnabled?: boolean
        iapAudience?: string
        iapAutoProvisioning?: boolean
        oidcAutoProvisioning?: boolean
        agentPrompt?: string
      }
      isConfigured?: boolean
      configured?: boolean
      setupCompleted?: boolean
      hasAdmin?: boolean
      adminConfigured?: boolean
      machineRuntime?: string
      internetPublicExposureDisabled?: boolean
      oidcEnabled?: boolean
      oidcIssuerUrl?: string
      oidcClientId?: string
      oidcClientSecretConfigured?: boolean
      oidcAllowedEmailDomains?: string[]
      serverDomain?: string
      baseDomain?: string
      domainPrefix?: string
      passwordLoginDisabled?: boolean
      iapEnabled?: boolean
      iapAudience?: string
      iapAutoProvisioning?: boolean
      oidcAutoProvisioning?: boolean
      agentPrompt?: string
    }>(
      ['/arca.v1.SetupService/GetSetupStatus', '/arca.v1.SetupService/GetStatus'],
      {},
    )

    const isConfigured =
      response.status?.completed ?? response.isConfigured ?? response.configured ?? response.setupCompleted ?? false
    const hasAdmin = response.status?.adminConfigured ?? response.hasAdmin ?? response.adminConfigured ?? false
    const internetPublicExposureDisabled =
      response.status?.internetPublicExposureDisabled ?? response.internetPublicExposureDisabled ?? false
    const oidcEnabled = response.status?.oidcEnabled ?? response.oidcEnabled ?? false
    const oidcIssuerURL = response.status?.oidcIssuerUrl ?? response.oidcIssuerUrl ?? ''
    const oidcClientID = response.status?.oidcClientId ?? response.oidcClientId ?? ''
    const oidcClientSecretConfigured =
      response.status?.oidcClientSecretConfigured ?? response.oidcClientSecretConfigured ?? false
    const oidcAllowedEmailDomains =
      response.status?.oidcAllowedEmailDomains ?? response.oidcAllowedEmailDomains ?? []
    const serverDomain = response.status?.serverDomain ?? response.serverDomain ?? ''
    const baseDomain = response.status?.baseDomain ?? response.baseDomain ?? ''
    const domainPrefix = response.status?.domainPrefix ?? response.domainPrefix ?? ''
    const passwordLoginDisabled =
      response.status?.passwordLoginDisabled ?? response.passwordLoginDisabled ?? false
    const iapEnabled = response.status?.iapEnabled ?? response.iapEnabled ?? false
    const iapAudience = response.status?.iapAudience ?? response.iapAudience ?? ''
    const iapAutoProvisioning = response.status?.iapAutoProvisioning ?? response.iapAutoProvisioning ?? false
    const oidcAutoProvisioning = response.status?.oidcAutoProvisioning ?? response.oidcAutoProvisioning ?? false
    const agentPrompt = response.status?.agentPrompt ?? response.agentPrompt ?? ''

    return {
      isConfigured,
      hasAdmin,
      baseDomain,
      domainPrefix,
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
      serverDomain,
      agentPrompt,
    }
  } catch (error) {
    if (error instanceof ApiError && (error.status === 404 || error.code.toLowerCase().includes('unimplemented'))) {
      return {
        isConfigured: true,
        hasAdmin: true,
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
        serverDomain: '',
        agentPrompt: '',
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

export async function setupComplete(
  adminEmail: string,
  adminPassword: string,
  serverDomain: string = '',
  setupPassword: string = '',
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
      serverExposureMethod: 2,
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
  serverDomain: string = '',
  passwordLoginDisabled: boolean = false,
  iapEnabled: boolean = false,
  iapAudience: string = '',
  iapAutoProvisioning: boolean = false,
  oidcAutoProvisioning: boolean = false,
  baseDomain: string = '',
  domainPrefix: string = '',
  agentPrompt: string = '',
): Promise<void> {
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
    serverExposureMethod: 2,
    serverDomain,
    passwordLoginDisabled,
    iapEnabled,
    iapAudience,
    iapAutoProvisioning,
    oidcAutoProvisioning,
    baseDomain,
    domainPrefix,
    agentPrompt,
  })
  if (response.status?.completed !== true) {
    throw new Error(response.message ?? 'failed to update domain settings')
  }
}

export async function listMachineExposures(machineID: string): Promise<MachineExposure[]> {
  const response = await exposureClient.listMachineExposures(
    create(ListMachineExposuresRequestSchema, { machineId: machineID }),
  )
  return response.exposures
}

export async function getMachineSharing(machineID: string): Promise<{
  members: MachineSharingMember[]
  generalAccess: GeneralAccess | undefined
  groups: MachineSharingGroup[]
}> {
  const response = await sharingClient.getMachineSharing(
    create(GetMachineSharingRequestSchema, { machineId: machineID }),
  )
  return {
    members: response.members,
    generalAccess: response.generalAccess,
    groups: response.groups,
  }
}

export async function searchUsers(query: string, limit = 10): Promise<{ id: string; email: string }[]> {
  const response = await userClient.searchUsers(
    create(SearchUsersRequestSchema, { query, limit }),
  )
  return response.users.map((u) => ({ id: u.id, email: u.email }))
}

export async function updateMachineSharing(
  machineID: string,
  members: { userId: string; email: string; role: string }[],
  generalAccess: { scope: string; role: string },
  groups?: { groupId: string; name: string; role: string }[],
): Promise<{
  members: MachineSharingMember[]
  generalAccess: GeneralAccess | undefined
  groups: MachineSharingGroup[]
}> {
  const response = await sharingClient.updateMachineSharing(
    create(UpdateMachineSharingRequestSchema, {
      machineId: machineID,
      members,
      generalAccess,
      groups: groups ?? [],
    }),
  )
  return {
    members: response.members,
    generalAccess: response.generalAccess,
    groups: response.groups,
  }
}

export async function requestMachineAccess(machineID: string, message?: string): Promise<void> {
  await sharingClient.requestMachineAccess(
    create(RequestMachineAccessRequestSchema, {
      machineId: machineID,
      message: message ?? '',
    }),
  )
}

export async function listMachineAccessRequests(machineID: string): Promise<MachineAccessRequest[]> {
  const response = await sharingClient.listMachineAccessRequests(
    create(ListMachineAccessRequestsRequestSchema, { machineId: machineID }),
  )
  return response.requests
}

export async function resolveMachineAccessRequest(requestID: string, action: string, role: string): Promise<void> {
  await sharingClient.resolveMachineAccessRequest(
    create(ResolveMachineAccessRequestRequestSchema, {
      requestId: requestID,
      action,
      role,
    }),
  )
}

// Group API
export async function listGroups(): Promise<UserGroup[]> {
  const response = await groupClient.listGroups(create(ListGroupsRequestSchema))
  return response.groups
}

export async function createGroup(name: string): Promise<UserGroup> {
  const response = await groupClient.createGroup(create(CreateGroupRequestSchema, { name }))
  if (response.group == null) {
    throw new Error('request failed')
  }
  return response.group
}

export async function deleteGroup(groupID: string): Promise<void> {
  await groupClient.deleteGroup(create(DeleteGroupRequestSchema, { groupId: groupID }))
}

export async function getGroup(groupID: string): Promise<{ group: UserGroup; members: UserGroupMember[] }> {
  const response = await groupClient.getGroup(create(GetGroupRequestSchema, { groupId: groupID }))
  if (response.group == null) {
    throw new Error('request failed')
  }
  return { group: response.group, members: response.members }
}

export async function addGroupMember(groupID: string, userID: string): Promise<void> {
  await groupClient.addGroupMember(create(AddGroupMemberRequestSchema, { groupId: groupID, userId: userID }))
}

export async function removeGroupMember(groupID: string, userID: string): Promise<void> {
  await groupClient.removeGroupMember(create(RemoveGroupMemberRequestSchema, { groupId: groupID, userId: userID }))
}

export async function searchGroups(query: string, limit = 10): Promise<UserGroup[]> {
  const response = await groupClient.searchGroups(create(SearchGroupsRequestSchema, { query, limit }))
  return response.groups
}

export type SlackConfigData = {
  enabled: boolean
  botToken: string
  defaultChannelId: string
  botTokenConfigured: boolean
}

export async function getSlackConfig(): Promise<SlackConfigData> {
  const response = await notificationClient.getSlackConfig(create(GetSlackConfigRequestSchema))
  return {
    enabled: response.config?.enabled ?? false,
    botToken: response.config?.botToken ?? '',
    defaultChannelId: response.config?.defaultChannelId ?? '',
    botTokenConfigured: response.config?.botTokenConfigured ?? false,
  }
}

export async function updateSlackConfig(config: { enabled: boolean; botToken: string; defaultChannelId: string }): Promise<SlackConfigData> {
  const response = await notificationClient.updateSlackConfig(
    create(UpdateSlackConfigRequestSchema, {
      config: {
        enabled: config.enabled,
        botToken: config.botToken,
        defaultChannelId: config.defaultChannelId,
      },
    }),
  )
  return {
    enabled: response.config?.enabled ?? false,
    botToken: response.config?.botToken ?? '',
    defaultChannelId: response.config?.defaultChannelId ?? '',
    botTokenConfigured: response.config?.botTokenConfigured ?? false,
  }
}

export async function testSlackNotification(channelId?: string): Promise<void> {
  await notificationClient.testSlackNotification(
    create(TestSlackNotificationRequestSchema, { channelId: channelId ?? '' }),
  )
}

export type UserNotificationSettingsData = {
  slackEnabled: boolean
  slackUserId: string
  slackAdminEnabled: boolean
}

export async function getUserNotificationSettings(): Promise<UserNotificationSettingsData> {
  const response = await notificationClient.getUserNotificationSettings(create(GetUserNotificationSettingsRequestSchema))
  return {
    slackEnabled: response.settings?.slackEnabled ?? true,
    slackUserId: response.settings?.slackUserId ?? '',
    slackAdminEnabled: response.slackAdminEnabled,
  }
}

export async function updateUserNotificationSettings(settings: { slackEnabled: boolean; slackUserId: string }): Promise<UserNotificationSettingsData> {
  const response = await notificationClient.updateUserNotificationSettings(
    create(UpdateUserNotificationSettingsRequestSchema, {
      settings: {
        slackEnabled: settings.slackEnabled,
        slackUserId: settings.slackUserId,
      },
    }),
  )
  return {
    slackEnabled: response.settings?.slackEnabled ?? true,
    slackUserId: response.settings?.slackUserId ?? '',
  }
}

// LLM Model API
export type { LLMModel }

export async function listUserLLMModels(): Promise<LLMModel[]> {
  const response = await userClient.listUserLLMModels(create(ListUserLLMModelsRequestSchema))
  return response.models
}

export async function createUserLLMModel(params: {
  configName: string
  endpointType: string
  customEndpoint: string
  modelName: string
  apiKey: string
  maxContextTokens: number
}): Promise<LLMModel> {
  const response = await userClient.createUserLLMModel(
    create(CreateUserLLMModelRequestSchema, {
      configName: params.configName,
      endpointType: params.endpointType,
      customEndpoint: params.customEndpoint,
      modelName: params.modelName,
      apiKey: params.apiKey,
      maxContextTokens: params.maxContextTokens,
    }),
  )
  if (response.model == null) {
    throw new Error('request failed')
  }
  return response.model
}

export async function updateUserLLMModel(params: {
  id: string
  configName: string
  endpointType: string
  customEndpoint: string
  modelName: string
  apiKey: string
  maxContextTokens: number
}): Promise<LLMModel> {
  const response = await userClient.updateUserLLMModel(
    create(UpdateUserLLMModelRequestSchema, {
      id: params.id,
      configName: params.configName,
      endpointType: params.endpointType,
      customEndpoint: params.customEndpoint,
      modelName: params.modelName,
      apiKey: params.apiKey,
      maxContextTokens: params.maxContextTokens,
    }),
  )
  if (response.model == null) {
    throw new Error('request failed')
  }
  return response.model
}

export async function deleteUserLLMModel(id: string): Promise<void> {
  await userClient.deleteUserLLMModel(create(DeleteUserLLMModelRequestSchema, { id }))
}

export async function duplicateUserLLMModel(id: string): Promise<LLMModel> {
  const response = await userClient.duplicateUserLLMModel(create(DuplicateUserLLMModelRequestSchema, { id }))
  if (!response.model) {
    throw new Error('request failed')
  }
  return response.model
}

// User Startup Script API

export async function getUserStartupScript(): Promise<string> {
  const response = await userClient.getUserStartupScript(create(GetUserStartupScriptRequestSchema, {}))
  return response.startupScript
}

export async function updateUserStartupScript(script: string): Promise<string> {
  const response = await userClient.updateUserStartupScript(create(UpdateUserStartupScriptRequestSchema, { startupScript: script }))
  return response.startupScript
}

// User Agent Prompt API

export async function getUserAgentPrompt(): Promise<string> {
  const response = await userClient.getUserAgentPrompt(create(GetUserAgentPromptRequestSchema, {}))
  return response.agentPrompt ?? ''
}

export async function updateUserAgentPrompt(agentPrompt: string): Promise<string> {
  const response = await userClient.updateUserAgentPrompt(
    create(UpdateUserAgentPromptRequestSchema, { agentPrompt }),
  )
  return response.agentPrompt ?? ''
}

// Custom Image API
import {
  CreateCustomImageRequestSchema,
  DeleteCustomImageRequestSchema,
  ImageService,
  ListAvailableImagesRequestSchema,
  ListCustomImagesRequestSchema,
  UpdateCustomImageRequestSchema,
} from '@/gen/arca/v1/image_pb'
import type { CustomImage } from '@/gen/arca/v1/image_pb'

const imageClient = createClient(ImageService, connectTransport)

export type { CustomImage }

export async function listCustomImages(): Promise<CustomImage[]> {
  const response = await imageClient.listCustomImages(create(ListCustomImagesRequestSchema))
  return response.images
}

export async function createCustomImage(params: {
  name: string
  templateType: string
  data: Record<string, string>
  description: string
  templateIds: string[]
}): Promise<CustomImage> {
  const response = await imageClient.createCustomImage(
    create(CreateCustomImageRequestSchema, {
      name: params.name,
      templateType: params.templateType,
      data: params.data,
      description: params.description,
      templateIds: params.templateIds,
    }),
  )
  if (response.image == null) {
    throw new Error('request failed')
  }
  return response.image
}

export async function updateCustomImage(params: {
  id: string
  name: string
  templateType: string
  data: Record<string, string>
  description: string
  templateIds: string[]
  visibility?: string
}): Promise<CustomImage> {
  const response = await imageClient.updateCustomImage(
    create(UpdateCustomImageRequestSchema, {
      id: params.id,
      name: params.name,
      templateType: params.templateType,
      data: params.data,
      description: params.description,
      templateIds: params.templateIds,
      ...(params.visibility !== undefined ? { visibility: params.visibility } : {}),
    }),
  )
  if (response.image == null) {
    throw new Error('request failed')
  }
  return response.image
}

export async function deleteCustomImage(id: string): Promise<void> {
  await imageClient.deleteCustomImage(create(DeleteCustomImageRequestSchema, { id }))
}

export async function listAvailableImages(templateId: string): Promise<CustomImage[]> {
  const response = await imageClient.listAvailableImages(
    create(ListAvailableImagesRequestSchema, { templateId }),
  )
  return response.images
}

// Admin API
import {
  AdminService,
  CreateServerLLMModelRequestSchema,
  DeleteServerLLMModelRequestSchema,
  GetAdminViewModeRequestSchema,
  ListAuditLogsRequestSchema,
  ListServerLLMModelsRequestSchema,
  SetAdminViewModeRequestSchema,
  UpdateServerLLMModelRequestSchema,
} from '@/gen/arca/v1/admin_pb'
import type { AuditLog, ServerLLMModel } from '@/gen/arca/v1/admin_pb'

const adminClient = createClient(AdminService, connectTransport)

export type { AuditLog, ServerLLMModel }

import type { AdminViewMode } from '@/lib/types'

export async function setAdminViewMode(mode: string): Promise<void> {
  await adminClient.setAdminViewMode(create(SetAdminViewModeRequestSchema, { mode }))
}

export async function getAdminViewMode(): Promise<AdminViewMode> {
  const res = await adminClient.getAdminViewMode(create(GetAdminViewModeRequestSchema))
  return { mode: res.mode, isAdmin: res.isAdmin }
}

export type AuditLogListResult = {
  auditLogs: AuditLog[]
  totalCount: number
}

export async function listAuditLogs(params: {
  limit?: number
  offset?: number
  actionPrefix?: string
  actorEmail?: string
} = {}): Promise<AuditLogListResult> {
  const response = await adminClient.listAuditLogs(create(ListAuditLogsRequestSchema, {
    limit: params.limit ?? 100,
    offset: params.offset ?? 0,
    actionPrefix: params.actionPrefix ?? '',
    actorEmail: params.actorEmail ?? '',
  }))
  return { auditLogs: response.auditLogs, totalCount: response.totalCount }
}

// Server LLM Model API
export async function listServerLLMModels(): Promise<ServerLLMModel[]> {
  const response = await adminClient.listServerLLMModels(create(ListServerLLMModelsRequestSchema))
  return response.models
}

export async function createServerLLMModel(params: {
  configName: string
  endpointType: string
  customEndpoint: string
  modelName: string
  tokenCommand: string
  maxContextTokens: number
}): Promise<ServerLLMModel> {
  const response = await adminClient.createServerLLMModel(
    create(CreateServerLLMModelRequestSchema, {
      configName: params.configName,
      endpointType: params.endpointType,
      customEndpoint: params.customEndpoint,
      modelName: params.modelName,
      tokenCommand: params.tokenCommand,
      maxContextTokens: params.maxContextTokens,
    }),
  )
  if (response.model == null) {
    throw new Error('request failed')
  }
  return response.model
}

export async function updateServerLLMModel(params: {
  id: string
  configName: string
  endpointType: string
  customEndpoint: string
  modelName: string
  tokenCommand: string
  maxContextTokens: number
}): Promise<ServerLLMModel> {
  const response = await adminClient.updateServerLLMModel(
    create(UpdateServerLLMModelRequestSchema, {
      id: params.id,
      configName: params.configName,
      endpointType: params.endpointType,
      customEndpoint: params.customEndpoint,
      modelName: params.modelName,
      tokenCommand: params.tokenCommand,
      maxContextTokens: params.maxContextTokens,
    }),
  )
  if (response.model == null) {
    throw new Error('request failed')
  }
  return response.model
}

export async function deleteServerLLMModel(id: string): Promise<void> {
  await adminClient.deleteServerLLMModel(create(DeleteServerLLMModelRequestSchema, { id }))
}
