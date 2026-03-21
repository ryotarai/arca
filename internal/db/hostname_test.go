package db

import "testing"

func TestMachineHostname(t *testing.T) {
	tests := []struct {
		prefix, name, baseDomain, want string
	}{
		{"arca", "myvm", "example.com", "arcamyvm.example.com"},
		{"", "myvm", "example.com", "myvm.example.com"},
		{"dev-", "test-1", "app.io", "dev-test-1.app.io"},
	}
	for _, tt := range tests {
		got := MachineHostname(tt.prefix, tt.name, tt.baseDomain)
		if got != tt.want {
			t.Errorf("MachineHostname(%q, %q, %q) = %q, want %q", tt.prefix, tt.name, tt.baseDomain, got, tt.want)
		}
	}
}

func TestExtractMachineNameFromHostname(t *testing.T) {
	tests := []struct {
		hostname, prefix, baseDomain string
		wantName                     string
		wantOK                       bool
	}{
		{"arcamyvm.example.com", "arca", "example.com", "myvm", true},
		{"myvm.example.com", "", "example.com", "myvm", true},
		{"dev-test-1.app.io", "dev-", "app.io", "test-1", true},
		{"unrelated.other.com", "arca", "example.com", "", false},
		{"arca.example.com", "arca", "example.com", "", false}, // empty name
		{"arcamyvm.example.com", "arca", "other.com", "", false},
	}
	for _, tt := range tests {
		name, ok := ExtractMachineNameFromHostname(tt.hostname, tt.prefix, tt.baseDomain)
		if ok != tt.wantOK || name != tt.wantName {
			t.Errorf("ExtractMachineNameFromHostname(%q, %q, %q) = (%q, %v), want (%q, %v)",
				tt.hostname, tt.prefix, tt.baseDomain, name, ok, tt.wantName, tt.wantOK)
		}
	}
}
