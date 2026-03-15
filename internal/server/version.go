package server

import (
	"os"
	"strings"

	"github.com/ryotarai/arca/internal/db"
	"github.com/ryotarai/arca/internal/version"
)

func currentSetupVersion() string {
	if value := strings.TrimSpace(os.Getenv("ARCA_SETUP_VERSION")); value != "" {
		return value
	}
	return version.Version
}

func machineUpdateRequired(machine db.Machine) bool {
	current := currentSetupVersion()
	setup := strings.TrimSpace(machine.SetupVersion)
	if setup == "" || current == "" {
		return false
	}
	return setup != current
}
