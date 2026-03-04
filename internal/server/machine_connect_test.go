package server

import (
	"errors"
	"testing"

	"github.com/ryotarai/arca/internal/cloudflare"
)

func TestValidateMachineName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantError string
	}{
		{name: "valid simple", input: "app1"},
		{name: "valid hyphen", input: "my-machine-1"},
		{name: "empty", input: "", wantError: "name is required"},
		{name: "too short", input: "ab", wantError: "name must be at least 3 characters"},
		{name: "dot not allowed", input: "my.machine", wantError: "name must use lowercase letters, digits, and hyphens only, and cannot start or end with a hyphen"},
		{name: "uppercase not allowed", input: "MyMachine", wantError: "name must use lowercase letters, digits, and hyphens only, and cannot start or end with a hyphen"},
		{name: "leading hyphen", input: "-machine", wantError: "name must use lowercase letters, digits, and hyphens only, and cannot start or end with a hyphen"},
		{name: "trailing hyphen", input: "machine-", wantError: "name must use lowercase letters, digits, and hyphens only, and cannot start or end with a hyphen"},
		{name: "reserved admin", input: "admin", wantError: "name is reserved"},
		{name: "reserved console", input: "console", wantError: "name is reserved"},
		{name: "reserved dash", input: "dash", wantError: "name is reserved"},
		{name: "reserved api", input: "api", wantError: "name is reserved"},
		{name: "reserved system", input: "system", wantError: "name is reserved"},
		{name: "reserved arca prefix", input: "arca-demo", wantError: "name cannot start with arca-"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateMachineName(tt.input)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error %q, got nil", tt.wantError)
			}
			if err.Error() != tt.wantError {
				t.Fatalf("unexpected error: got %q want %q", err.Error(), tt.wantError)
			}
		})
	}
}

func TestIsActiveTunnelConnectionError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  cloudflare.APIError
		want bool
	}{
		{
			name: "matches code 1022",
			err: cloudflare.APIError{
				Code:    1022,
				Message: "any message",
			},
			want: true,
		},
		{
			name: "matches message",
			err: cloudflare.APIError{
				Code:    0,
				Message: "This tunnel has active connections.",
			},
			want: true,
		},
		{
			name: "does not match",
			err: cloudflare.APIError{
				Code:    1003,
				Message: "resource not found",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isActiveTunnelConnectionError(tt.err)
			if got != tt.want {
				t.Fatalf("isActiveTunnelConnectionError(%+v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsActiveTunnelConnectionDeleteError(t *testing.T) {
	t.Parallel()

	activeErr := cloudflare.APIError{Code: 1022, Message: "active connections"}
	if !isActiveTunnelConnectionDeleteError(activeErr) {
		t.Fatalf("expected active connection delete error to be detected")
	}

	notActiveErr := cloudflare.APIError{Code: 1003, Message: "not found"}
	if isActiveTunnelConnectionDeleteError(notActiveErr) {
		t.Fatalf("unexpected detection for non-active error")
	}

	if isActiveTunnelConnectionDeleteError(errors.New("network timeout")) {
		t.Fatalf("unexpected detection for non-cloudflare error")
	}
}
