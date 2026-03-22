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

// GetProfileStartupScript extracts the startup_script from a profile config
// JSON by looking inside each provider sub-object (libvirt, gce, lxd).
// It checks both camelCase (startupScript) and snake_case (startup_script)
// keys. Returns empty string if not set.
func GetProfileStartupScript(configJSON string) string {
	if configJSON == "" || configJSON == "{}" {
		return ""
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(configJSON), &raw); err != nil {
		return ""
	}
	for _, provider := range []string{"libvirt", "gce", "lxd"} {
		providerRaw, ok := raw[provider]
		if !ok {
			continue
		}
		var providerConfig map[string]json.RawMessage
		if err := json.Unmarshal(providerRaw, &providerConfig); err != nil {
			continue
		}
		// Check camelCase first, then snake_case
		for _, key := range []string{"startupScript", "startup_script"} {
			if scriptRaw, ok := providerConfig[key]; ok {
				var script string
				if err := json.Unmarshal(scriptRaw, &script); err == nil && strings.TrimSpace(script) != "" {
					return script
				}
			}
		}
	}
	return ""
}
