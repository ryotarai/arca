package db

import (
	"encoding/json"
	"strings"
)

const (
	MachineExposureMethodCloudflareTunnel = "cloudflare_tunnel"
	MachineExposureMethodProxyViaServer   = "proxy_via_server"
)

// RuntimeExposureConfig represents the exposure configuration stored
// inside a runtime catalog entry's config_json.
type RuntimeExposureConfig struct {
	Method              string `json:"method,omitempty"`
	DomainPrefix        string `json:"domainPrefix,omitempty"`
	BaseDomain          string `json:"baseDomain,omitempty"`
	CloudflareAPIToken  string `json:"cloudflareApiToken,omitempty"`
	CloudflareAccountID string `json:"cloudflareAccountId,omitempty"`
	CloudflareZoneID    string `json:"cloudflareZoneId,omitempty"`
	Connectivity        string `json:"connectivity,omitempty"`
}

// GetRuntimeExposureMethod extracts the machine exposure method from
// a runtime config JSON string. Returns cloudflare_tunnel as default.
func GetRuntimeExposureMethod(configJSON string) string {
	cfg := GetRuntimeExposureConfig(configJSON)
	method := strings.ToLower(strings.TrimSpace(cfg.Method))
	// Match both plain value ("proxy_via_server") and protobuf enum name
	// ("MACHINE_EXPOSURE_METHOD_PROXY_VIA_SERVER") since config_json is
	// serialized via protojson which uses the enum name.
	if method == MachineExposureMethodProxyViaServer || strings.HasSuffix(method, "_proxy_via_server") {
		return MachineExposureMethodProxyViaServer
	}
	return MachineExposureMethodCloudflareTunnel
}

// GetRuntimeExposureConfig extracts the exposure config from a runtime
// config JSON string. The exposure config is stored in the "exposure" key.
func GetRuntimeExposureConfig(configJSON string) RuntimeExposureConfig {
	var wrapper struct {
		Exposure RuntimeExposureConfig `json:"exposure"`
	}
	if err := json.Unmarshal([]byte(configJSON), &wrapper); err != nil {
		return RuntimeExposureConfig{}
	}
	return wrapper.Exposure
}
