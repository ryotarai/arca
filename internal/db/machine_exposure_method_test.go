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
