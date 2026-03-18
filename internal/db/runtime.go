package db

import "strings"

const (
	MachineRuntimeLibvirt = "libvirt"
)

func NormalizeMachineTemplate(runtime string) string {
	return strings.TrimSpace(runtime)
}
