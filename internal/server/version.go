package server

import (
	"os"
	"runtime/debug"
	"strings"

	"github.com/ryotarai/arca/internal/db"
)

func currentSetupVersion() string {
	if value := strings.TrimSpace(os.Getenv("ARCA_SETUP_VERSION")); value != "" {
		return value
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		version := strings.TrimSpace(info.Main.Version)
		if version != "" && version != "(devel)" {
			return version
		}
	}
	return "dev"
}

func machineUpdateRequired(machine db.Machine) bool {
	current := currentSetupVersion()
	setup := strings.TrimSpace(machine.SetupVersion)
	if setup == "" || current == "" {
		return false
	}
	return setup != current
}
