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

func TestValidateOIDCIssuerURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty allowed", input: "", want: ""},
		{name: "normalizes", input: " https://accounts.google.com/ ", want: "https://accounts.google.com"},
		{name: "rejects non-https", input: "http://accounts.google.com", wantErr: true},
		{name: "rejects invalid", input: "not-a-url", wantErr: true},
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

func TestNormalizeOIDCAllowedEmailDomains(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   []string
		want    []string
		wantErr bool
	}{
		{name: "empty", input: nil, want: nil},
		{name: "normalizes and deduplicates", input: []string{" Example.COM ", "example.com", "corp.dev"}, want: []string{"example.com", "corp.dev"}},
		{name: "rejects email", input: []string{"user@example.com"}, wantErr: true},
		{name: "rejects invalid", input: []string{"bad-domain"}, wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeOIDCAllowedEmailDomains(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeOIDCAllowedEmailDomains(%v) expected error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeOIDCAllowedEmailDomains(%v) unexpected error: %v", tt.input, err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("normalizeOIDCAllowedEmailDomains(%v) len=%d, want %d", tt.input, len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("normalizeOIDCAllowedEmailDomains(%v) item[%d]=%q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}
