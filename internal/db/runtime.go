package db

import "strings"

const (
	MachineRuntimeDocker  = "docker"
	MachineRuntimeLibvirt = "libvirt"
)

func NormalizeMachineRuntime(runtime string) string {
	switch strings.ToLower(strings.TrimSpace(runtime)) {
	case MachineRuntimeLibvirt:
		return MachineRuntimeLibvirt
	case MachineRuntimeDocker:
		return MachineRuntimeDocker
	default:
		return MachineRuntimeDocker
	}
}

func IsSupportedMachineRuntime(runtime string) bool {
	switch NormalizeMachineRuntime(runtime) {
	case MachineRuntimeDocker, MachineRuntimeLibvirt:
		return true
	default:
		return false
	}
}
