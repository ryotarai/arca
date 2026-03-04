package server

import "testing"

func TestValidateBaseDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "valid domain",
			input: "arca.dev",
			want:  "arca.dev",
		},
		{
			name:  "normalizes casing and whitespace",
			input: "  ArCa.DEV  ",
			want:  "arca.dev",
		},
		{
			name:    "rejects url",
			input:   "https://arca.dev",
			wantErr: true,
		},
		{
			name:    "rejects invalid chars",
			input:   "arca_dev",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := validateBaseDomain(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("validateBaseDomain(%q) expected error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateBaseDomain(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("validateBaseDomain(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateDomainPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		expect  string
		wantErr bool
	}{
		{
			name:   "keeps trailing hyphen for separator usage",
			input:  "arca-",
			expect: "arca-",
		},
		{
			name:   "normalizes casing and whitespace",
			input:  "  ArCa-  ",
			expect: "arca-",
		},
		{
			name:    "rejects invalid characters",
			input:   "arca_+foo",
			wantErr: true,
		},
		{
			name:   "keeps internal hyphens",
			input:  "pre-fix",
			expect: "pre-fix",
		},
		{
			name:    "rejects too long prefix",
			input:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := validateDomainPrefix(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("validateDomainPrefix(%q) expected error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateDomainPrefix(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.expect {
				t.Fatalf("validateDomainPrefix(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}
