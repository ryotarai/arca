package db

import "strings"

const (
	MachineRuntimeLibvirt = "libvirt"
)

func NormalizeMachineRuntime(runtime string) string {
	runtime = strings.TrimSpace(runtime)
	if runtime == "" {
		return MachineRuntimeLibvirt
	}
	return runtime
}

func IsSupportedMachineRuntime(runtime string) bool {
	return strings.EqualFold(strings.TrimSpace(runtime), MachineRuntimeLibvirt)
}
