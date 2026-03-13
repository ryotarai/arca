package db

import "testing"

func TestGetRuntimeExposureMethod(t *testing.T) {
	tests := []struct {
		name       string
		configJSON string
		want       string
	}{
		{
			name:       "plain proxy_via_server value",
			configJSON: `{"exposure":{"method":"proxy_via_server"}}`,
			want:       MachineExposureMethodProxyViaServer,
		},
		{
			name:       "protobuf enum name MACHINE_EXPOSURE_METHOD_PROXY_VIA_SERVER",
			configJSON: `{"exposure":{"method":"MACHINE_EXPOSURE_METHOD_PROXY_VIA_SERVER"}}`,
			want:       MachineExposureMethodProxyViaServer,
		},
		{
			name:       "plain cloudflare_tunnel value",
			configJSON: `{"exposure":{"method":"cloudflare_tunnel"}}`,
			want:       MachineExposureMethodCloudflareTunnel,
		},
		{
			name:       "protobuf enum name MACHINE_EXPOSURE_METHOD_CLOUDFLARE_TUNNEL",
			configJSON: `{"exposure":{"method":"MACHINE_EXPOSURE_METHOD_CLOUDFLARE_TUNNEL"}}`,
			want:       MachineExposureMethodCloudflareTunnel,
		},
		{
			name:       "empty method defaults to cloudflare_tunnel",
			configJSON: `{"exposure":{}}`,
			want:       MachineExposureMethodCloudflareTunnel,
		},
		{
			name:       "missing exposure defaults to cloudflare_tunnel",
			configJSON: `{}`,
			want:       MachineExposureMethodCloudflareTunnel,
		},
		{
			name:       "invalid JSON defaults to cloudflare_tunnel",
			configJSON: `not json`,
			want:       MachineExposureMethodCloudflareTunnel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetRuntimeExposureMethod(tt.configJSON)
			if got != tt.want {
				t.Errorf("GetRuntimeExposureMethod() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetRuntimeAutoStopTimeoutSeconds(t *testing.T) {
	tests := []struct {
		name       string
		configJSON string
		want       int64
	}{
		{
			name:       "numeric value",
			configJSON: `{"autoStopTimeoutSeconds":86400}`,
			want:       86400,
		},
		{
			name:       "string value (protobuf int64 encoding)",
			configJSON: `{"autoStopTimeoutSeconds":"86400"}`,
			want:       86400,
		},
		{
			name:       "zero",
			configJSON: `{"autoStopTimeoutSeconds":0}`,
			want:       0,
		},
		{
			name:       "missing field",
			configJSON: `{}`,
			want:       0,
		},
		{
			name:       "invalid JSON",
			configJSON: `not json`,
			want:       0,
		},
		{
			name:       "non-numeric string",
			configJSON: `{"autoStopTimeoutSeconds":"abc"}`,
			want:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetRuntimeAutoStopTimeoutSeconds(tt.configJSON)
			if got != tt.want {
				t.Errorf("GetRuntimeAutoStopTimeoutSeconds() = %d, want %d", got, tt.want)
			}
		})
	}
}
