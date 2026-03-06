import { Machine as MachineMessage } from '@/gen/arca/v1/machine_pb'
import { type EndpointVisibility, type MachineExposure as MachineExposureMessage } from '@/gen/arca/v1/tunnel_pb'

export type User = {
  id: string
  email: string
}

export type ManagedUser = {
  id: string
  email: string
  setupRequired: boolean
  setupTokenExpiresAt: number
  createdAt: number
}

export type Machine = MachineMessage

export type MachineEvent = import('@/gen/arca/v1/machine_pb').MachineEvent

export type MachineExposure = MachineExposureMessage
export type MachineExposureVisibility = EndpointVisibility

export type SetupStatus = {
  isConfigured: boolean
  hasAdmin: boolean
  cloudflareZoneID: string
  baseDomain: string
  domainPrefix: string
  machineRuntime: 'libvirt'
  internetPublicExposureDisabled: boolean
  oidcEnabled: boolean
  oidcIssuerURL: string
  oidcClientID: string
  oidcClientSecretConfigured: boolean
  oidcAllowedEmailDomains: string[]
}
