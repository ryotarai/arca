package server

import "testing"

func TestNormalizeDomainPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		expect string
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
			name:   "drops invalid characters",
			input:  "arca_+foo",
			expect: "arcafoo",
		},
		{
			name:   "keeps internal hyphens",
			input:  "pre-fix",
			expect: "pre-fix",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeDomainPrefix(tt.input); got != tt.expect {
				t.Fatalf("normalizeDomainPrefix(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}
