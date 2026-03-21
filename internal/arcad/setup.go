package arcad

import (
	"context"
	"fmt"
	"log"
	"os"
)

// SetupConfig holds configuration for the idempotent setup phase.
type SetupConfig struct {
	DaemonUser           string
	InteractiveUser      string
AgentEndpointURL     string
	ShelleyBinaryURLBase string
	ShelleyBasePath      string
	ShelleyPort          string
	ShelleyDBPath        string
	TTydSocket           string
	TTydBasePath         string
	StartupSentinel      string
	UserStartupScript    string
}

// SetupConfigFromEnv reads setup configuration from environment variables.
func SetupConfigFromEnv() SetupConfig {
	return SetupConfig{
		DaemonUser:           envOrDefault("ARCA_DAEMON_USER", "arcad"),
		InteractiveUser:      envOrDefault("ARCA_INTERACTIVE_USER", "arcauser"),
AgentEndpointURL:     envOrDefault("ARCA_AGENT_ENDPOINT_URL", "http://localhost:11030"),
		ShelleyBinaryURLBase: os.Getenv("SHELLEY_BINARY_URL_BASE"),
		ShelleyBasePath:      envOrDefault("SHELLEY_BASE_PATH", "/__arca/shelley"),
		ShelleyPort:          envOrDefault("SHELLEY_PORT", "21032"),
		ShelleyDBPath:        envOrDefault("SHELLEY_DB_PATH", "/var/lib/arca/shelley/shelley.db"),
		TTydSocket:           envOrDefault("ARCAD_TTYD_SOCKET", "/run/arca/ttyd.sock"),
		TTydBasePath:         envOrDefault("TTYD_BASE_PATH", "/__arca/ttyd"),
		StartupSentinel:      envOrDefault("ARCAD_STARTUP_SENTINEL", "/var/lib/arca/startup.done"),
		UserStartupScript:    "/usr/local/bin/arca-user-startup.sh",
	}
}

func envOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// RunSetup runs the idempotent provisioning phase using Ansible playbooks.
func RunSetup(ctx context.Context, _ SetupConfig) error {
	log.Printf("setup: ensuring ansible is installed")
	if err := ensureAnsible(ctx); err != nil {
		return fmt.Errorf("ensure ansible: %w", err)
	}

	log.Printf("setup: running ansible playbook")
	if err := extractAndRunPlaybook(ctx); err != nil {
		return fmt.Errorf("ansible playbook: %w", err)
	}

	log.Printf("setup: complete")
	return nil
}
