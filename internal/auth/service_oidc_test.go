package auth

import "testing"

func TestValidateOIDCIssuerURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "normalizes", input: " https://accounts.google.com/ ", want: "https://accounts.google.com"},
		{name: "rejects empty", input: "", wantErr: true},
		{name: "rejects non https", input: "http://accounts.google.com", wantErr: true},
		{name: "rejects invalid", input: "%%%%", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := validateOIDCIssuerURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("validateOIDCIssuerURL(%q) expected error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateOIDCIssuerURL(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("validateOIDCIssuerURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsEmailDomainAllowed(t *testing.T) {
	t.Parallel()

	if !isEmailDomainAllowed("admin@example.com", nil) {
		t.Fatal("expected nil allowlist to allow email domain")
	}
	if isEmailDomainAllowed("admin@example.com", []string{"corp.dev"}) {
		t.Fatal("expected domain mismatch to be rejected")
	}
	if !isEmailDomainAllowed("admin@example.com", []string{"example.com"}) {
		t.Fatal("expected matching domain to pass")
	}
	if isEmailDomainAllowed("adminexample.com", []string{"example.com"}) {
		t.Fatal("expected malformed email to be rejected")
	}
}
