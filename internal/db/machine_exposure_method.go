package db

import (
	"encoding/json"
	"strings"
)


const (
	MachineExposureMethodProxyViaServer = "proxy_via_server"
)

// ProfileExposureConfig represents the exposure configuration stored
// inside a machine profile's config_json.
type ProfileExposureConfig struct {
	Method       string `json:"method,omitempty"`
	Connectivity string `json:"connectivity,omitempty"`
}

// GetProfileExposureMethod extracts the machine exposure method from
// a profile config JSON string. Always returns proxy_via_server.
func GetProfileExposureMethod(configJSON string) string {
	return MachineExposureMethodProxyViaServer
}

// GetProfileExposureConfig extracts the exposure config from a profile
// config JSON string. The exposure config is stored in the "exposure" key.
func GetProfileExposureConfig(configJSON string) ProfileExposureConfig {
	var wrapper struct {
		Exposure ProfileExposureConfig `json:"exposure"`
	}
	if err := json.Unmarshal([]byte(configJSON), &wrapper); err != nil {
		return ProfileExposureConfig{}
	}
	return wrapper.Exposure
}

// GetProfileConnectivity extracts the connectivity setting from a profile
// config JSON string. Returns empty string if not set.
func GetProfileConnectivity(configJSON string) string {
	return strings.TrimSpace(GetProfileExposureConfig(configJSON).Connectivity)
}

// GetProfileServerAPIURL extracts the server API URL override from a profile
// config JSON string. Returns empty string if not set.
func GetProfileServerAPIURL(configJSON string) string {
	var wrapper struct {
		ServerApiUrl string `json:"serverApiUrl,omitempty"`
	}
	if err := json.Unmarshal([]byte(configJSON), &wrapper); err != nil {
		return ""
	}
	return strings.TrimSpace(wrapper.ServerApiUrl)
}

// GetProfileAgentPrompt extracts the agent prompt from a profile config
// JSON string. Returns empty string if not set.
func GetProfileAgentPrompt(configJSON string) string {
	var wrapper struct {
		AgentPrompt string `json:"agentPrompt,omitempty"`
	}
	if err := json.Unmarshal([]byte(configJSON), &wrapper); err != nil {
		return ""
	}
	return strings.TrimSpace(wrapper.AgentPrompt)
}

// GetProfileAutoStopTimeoutSeconds extracts the auto-stop timeout from a
// profile config JSON string. Returns 0 if not set (disabled).
// The value may be stored as a JSON number or a quoted string (protobuf
// encodes int64 as string), so we accept both forms.
func GetProfileAutoStopTimeoutSeconds(configJSON string) int64 {
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
