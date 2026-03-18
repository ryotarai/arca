package db

import (
	"encoding/json"
	"strings"
)


const (
	MachineExposureMethodProxyViaServer = "proxy_via_server"
)

// TemplateExposureConfig represents the exposure configuration stored
// inside a machine template's config_json.
type TemplateExposureConfig struct {
	Method       string `json:"method,omitempty"`
	DomainPrefix string `json:"domainPrefix,omitempty"`
	BaseDomain   string `json:"baseDomain,omitempty"`
	Connectivity string `json:"connectivity,omitempty"`
}

// GetTemplateExposureMethod extracts the machine exposure method from
// a template config JSON string. Always returns proxy_via_server.
func GetTemplateExposureMethod(configJSON string) string {
	return MachineExposureMethodProxyViaServer
}

// GetTemplateExposureConfig extracts the exposure config from a template
// config JSON string. The exposure config is stored in the "exposure" key.
func GetTemplateExposureConfig(configJSON string) TemplateExposureConfig {
	var wrapper struct {
		Exposure TemplateExposureConfig `json:"exposure"`
	}
	if err := json.Unmarshal([]byte(configJSON), &wrapper); err != nil {
		return TemplateExposureConfig{}
	}
	return wrapper.Exposure
}

// GetTemplateConnectivity extracts the connectivity setting from a template
// config JSON string. Returns empty string if not set.
func GetTemplateConnectivity(configJSON string) string {
	return strings.TrimSpace(GetTemplateExposureConfig(configJSON).Connectivity)
}

// GetTemplateServerAPIURL extracts the server API URL override from a template
// config JSON string. Returns empty string if not set.
func GetTemplateServerAPIURL(configJSON string) string {
	var wrapper struct {
		ServerApiUrl string `json:"serverApiUrl,omitempty"`
	}
	if err := json.Unmarshal([]byte(configJSON), &wrapper); err != nil {
		return ""
	}
	return strings.TrimSpace(wrapper.ServerApiUrl)
}

// GetTemplateAutoStopTimeoutSeconds extracts the auto-stop timeout from a
// template config JSON string. Returns 0 if not set (disabled).
// The value may be stored as a JSON number or a quoted string (protobuf
// encodes int64 as string), so we accept both forms.
func GetTemplateAutoStopTimeoutSeconds(configJSON string) int64 {
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
