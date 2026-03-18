import { Machine as MachineMessage } from '@/gen/arca/v1/machine_pb'
import { type MachineExposure as MachineExposureMessage } from '@/gen/arca/v1/tunnel_pb'

export type User = {
  id: string
  email: string
  role: string
}

export type ManagedUser = {
  id: string
  email: string
  setupRequired: boolean
  role: string
  setupTokenExpiresAt: number
  createdAt: number
}

export type UserSettings = {
  sshPublicKeys: string[]
}

export type ImpersonationStatus = {
  isImpersonating: boolean
  impersonatedUserEmail: string
  originalUserEmail: string
}

export type Machine = MachineMessage

export type MachineEvent = import('@/gen/arca/v1/machine_pb').MachineEvent

export type MachineExposure = MachineExposureMessage

export type ServerExposureMethod = 'cloudflare_tunnel' | 'manual'

export type SetupStatus = {
  isConfigured: boolean
  hasAdmin: boolean
  cloudflareZoneID: string
  baseDomain: string
  domainPrefix: string
  internetPublicExposureDisabled: boolean
  oidcEnabled: boolean
  oidcIssuerURL: string
  oidcClientID: string
  oidcClientSecretConfigured: boolean
  oidcAllowedEmailDomains: string[]
  passwordLoginDisabled: boolean
  iapEnabled: boolean
  iapAudience: string
  iapAutoProvisioning: boolean
  oidcAutoProvisioning: boolean
  serverExposureMethod: ServerExposureMethod
  serverDomain: string
}

export type MachineExposureMethodType = 'cloudflare_tunnel' | 'proxy_via_server'

export type MachineExposureConfig = {
  method: MachineExposureMethodType
  domainPrefix: string
  baseDomain: string
  cloudflareApiToken: string
  cloudflareAccountId: string
  cloudflareZoneId: string
  connectivity: 'private_ip' | 'public_ip' | ''
}

export type RuntimeCatalogType = 'libvirt' | 'gce' | 'lxd'

export type RuntimeCatalogConfig =
  | {
      type: 'libvirt'
      uri: string
      network: string
      storagePool: string
      startupScript: string
    }
  | {
      type: 'gce'
      project: string
      zone: string
      network: string
      subnetwork: string
      serviceAccountEmail: string
      startupScript: string
      machineType: string
      diskSizeGb: number
      imageProject: string
      imageFamily: string
      allowedMachineTypes: string[]
    }
  | {
      type: 'lxd'
      endpoint: string
      startupScript: string
    }

export type RuntimeSummary = {
  id: string
  name: string
  type: RuntimeCatalogType
}

export type RuntimeCatalogItem = {
  id: string
  name: string
  type: RuntimeCatalogType
  config: RuntimeCatalogConfig
  exposure: MachineExposureConfig
  serverApiUrl: string
  autoStopTimeoutSeconds: number
  createdAt: number
  updatedAt: number
}
