package server

import "testing"

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
