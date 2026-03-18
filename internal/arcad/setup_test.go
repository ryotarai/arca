package arcad

import (
	"testing"
)

func TestSetupConfigFromEnv_Defaults(t *testing.T) {
	cfg := SetupConfigFromEnv()
	if cfg.DaemonUser != "arcad" {
		t.Fatalf("DaemonUser = %q, want %q", cfg.DaemonUser, "arcad")
	}
	if cfg.InteractiveUser != "arcauser" {
		t.Fatalf("InteractiveUser = %q, want %q", cfg.InteractiveUser, "arcauser")
	}
	if cfg.ShelleyPort != "21032" {
		t.Fatalf("ShelleyPort = %q, want %q", cfg.ShelleyPort, "21032")
	}
}
