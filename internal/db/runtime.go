package db

import "strings"

const (
	MachineRuntimeLibvirt = "libvirt"
)

func NormalizeMachineRuntime(runtime string) string {
	return strings.TrimSpace(runtime)
}
