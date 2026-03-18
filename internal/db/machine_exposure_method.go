package db

import (
	"encoding/json"
	"strings"
)


const (
	MachineExposureMethodProxyViaServer = "proxy_via_server"
)

// RuntimeExposureConfig represents the exposure configuration stored
// inside a runtime catalog entry's config_json.
type RuntimeExposureConfig struct {
	Method       string `json:"method,omitempty"`
	DomainPrefix string `json:"domainPrefix,omitempty"`
	BaseDomain   string `json:"baseDomain,omitempty"`
	Connectivity string `json:"connectivity,omitempty"`
}

// GetRuntimeExposureMethod extracts the machine exposure method from
// a runtime config JSON string. Always returns proxy_via_server.
func GetRuntimeExposureMethod(configJSON string) string {
	return MachineExposureMethodProxyViaServer
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

// GetRuntimeConnectivity extracts the connectivity setting from a runtime
// config JSON string. Returns empty string if not set.
func GetRuntimeConnectivity(configJSON string) string {
	return strings.TrimSpace(GetRuntimeExposureConfig(configJSON).Connectivity)
}

// GetRuntimeServerAPIURL extracts the server API URL override from a runtime
// config JSON string. Returns empty string if not set.
func GetRuntimeServerAPIURL(configJSON string) string {
	var wrapper struct {
		ServerApiUrl string `json:"serverApiUrl,omitempty"`
	}
	if err := json.Unmarshal([]byte(configJSON), &wrapper); err != nil {
		return ""
	}
	return strings.TrimSpace(wrapper.ServerApiUrl)
}

// GetRuntimeAutoStopTimeoutSeconds extracts the auto-stop timeout from a
// runtime config JSON string. Returns 0 if not set (disabled).
// The value may be stored as a JSON number or a quoted string (protobuf
// encodes int64 as string), so we accept both forms.
func GetRuntimeAutoStopTimeoutSeconds(configJSON string) int64 {
	var wrapper struct {
		AutoStopTimeoutSeconds json.Number `json:"autoStopTimeoutSeconds,omitempty"`
	}
	if err := json.Unmarshal([]byte(configJSON), &wrapper); err != nil {
		return 0
	}
	v, err := wrapper.AutoStopTimeoutSeconds.Int64()
	if err != nil {
		return 0
	}
	return v
}
