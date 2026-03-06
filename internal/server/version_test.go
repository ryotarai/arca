package server

import (
	"testing"

	"github.com/ryotarai/arca/internal/db"
)

func TestMachineUpdateRequired(t *testing.T) {
	tests := []struct {
		name         string
		setupVersion string
		envVersion   string
		want         bool
	}{
		{
			name:         "matching version",
			setupVersion: "v1.2.3",
			envVersion:   "v1.2.3",
			want:         false,
		},
		{
			name:         "mismatched version",
			setupVersion: "v1.2.2",
			envVersion:   "v1.2.3",
			want:         true,
		},
		{
			name:         "blank setup version",
			setupVersion: "   ",
			envVersion:   "v1.2.3",
			want:         false,
		},
		{
			name:         "env override is used",
			setupVersion: "override-version",
			envVersion:   "  override-version  ",
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ARCA_SETUP_VERSION", tt.envVersion)

			got := machineUpdateRequired(db.Machine{SetupVersion: tt.setupVersion})
			if got != tt.want {
				t.Fatalf("machineUpdateRequired(%q) = %v, want %v", tt.setupVersion, got, tt.want)
			}
		})
	}
}
