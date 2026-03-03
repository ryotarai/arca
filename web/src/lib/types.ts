import { Machine as MachineMessage } from '@/gen/arca/v1/machine_pb'

export type User = {
  id: string
  email: string
}

export type Machine = MachineMessage

export type SetupStatus = {
  isConfigured: boolean
  hasAdmin: boolean
  cloudflareZoneID: string
  baseDomain: string
  domainPrefix: string
  machineRuntime: 'docker' | 'libvirt'
}
