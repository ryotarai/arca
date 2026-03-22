package db

import "strings"

const (
	MachineRuntimeLibvirt = "libvirt"
)

func NormalizeMachineProfile(runtime string) string {
	return strings.TrimSpace(runtime)
}

// NormalizeMachineTemplate is an alias for backward compatibility.
// Deprecated: Use NormalizeMachineProfile instead.
func NormalizeMachineTemplate(runtime string) string {
	return NormalizeMachineProfile(runtime)
}
