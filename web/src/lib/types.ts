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

export type AdminViewMode = {
  mode: string
  isAdmin: boolean
}

export type Machine = MachineMessage

export type MachineEvent = import('@/gen/arca/v1/machine_pb').MachineEvent

export type MachineExposure = MachineExposureMessage

export type SetupStatus = {
  isConfigured: boolean
  hasAdmin: boolean
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
  serverDomain: string
}

export type MachineExposureMethodType = 'proxy_via_server'

export type MachineExposureConfig = {
  method: MachineExposureMethodType
  domainPrefix: string
  baseDomain: string
  connectivity: 'private_ip' | 'public_ip' | ''
}

export type MachineTemplateType = 'libvirt' | 'gce' | 'lxd'

export type MachineTemplateConfig =
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
      diskSizeGb: number
      allowedMachineTypes: string[]
    }
  | {
      type: 'lxd'
      endpoint: string
      startupScript: string
    }

export type MachineTemplateSummary = {
  id: string
  name: string
  type: MachineTemplateType
  allowedMachineTypes: string[]
}

export type MachineTemplateItem = {
  id: string
  name: string
  type: MachineTemplateType
  config: MachineTemplateConfig
  exposure: MachineExposureConfig
  serverApiUrl: string
  autoStopTimeoutSeconds: number
  createdAt: number
  updatedAt: number
}
