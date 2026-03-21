package db

import "strings"

// MachineHostname computes the full hostname for a machine from setup_state config.
// Format: {prefix}{machineName}.{baseDomain}
func MachineHostname(prefix, machineName, baseDomain string) string {
	return prefix + machineName + "." + baseDomain
}

// ExtractMachineNameFromHostname parses a hostname and extracts the machine name
// by stripping the base_domain suffix and domain_prefix prefix.
// Returns ("", false) if the hostname does not match the expected pattern.
func ExtractMachineNameFromHostname(hostname, prefix, baseDomain string) (string, bool) {
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	suffix := "." + baseDomain
	if !strings.HasSuffix(hostname, suffix) {
		return "", false
	}
	subdomain := strings.TrimSuffix(hostname, suffix)
	if !strings.HasPrefix(subdomain, prefix) {
		return "", false
	}
	name := strings.TrimPrefix(subdomain, prefix)
	if name == "" {
		return "", false
	}
	return name, true
}
