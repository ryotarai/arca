package db

import "strings"

const (
	MachineRuntimeLibvirt = "libvirt"
)

func NormalizeMachineRuntime(runtime string) string {
	if strings.EqualFold(strings.TrimSpace(runtime), MachineRuntimeLibvirt) {
		return MachineRuntimeLibvirt
	}
	return MachineRuntimeLibvirt
}

func IsSupportedMachineRuntime(runtime string) bool {
	return NormalizeMachineRuntime(runtime) == MachineRuntimeLibvirt
}
