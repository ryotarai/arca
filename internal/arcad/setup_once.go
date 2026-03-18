package arcad

import (
	"context"
	"fmt"
	"log"
)

// RunSetupOnce runs the idempotent provisioning steps that install software
// and configure the environment, but skips steps that start services or
// require a running control plane. This is designed for Packer image builds
// where dependencies should be pre-installed.
func RunSetupOnce(ctx context.Context, setupCfg SetupConfig) error {
	steps := []struct {
		name string
		fn   func(context.Context, SetupConfig) error
	}{
		{"create directories", stepCreateDirectories},
		{"create users and groups", stepCreateUsersAndGroups},
		{"install system packages", stepInstallPackages},
		{"configure sudoers", stepConfigureSudoers},
		{"configure workspace", stepConfigureWorkspace},
		{"download cloudflared", stepDownloadCloudflared},
		{"download shelley", stepDownloadShelley},
		{"create shelley data dir", stepCreateShelleyDataDir},
		{"install dev tools", stepInstallDevTools},
	}

	for _, step := range steps {
		log.Printf("setup-once: %s", step.name)
		if err := step.fn(ctx, setupCfg); err != nil {
			return fmt.Errorf("setup-once step %q failed: %w", step.name, err)
		}
	}

	log.Printf("setup-once: complete")
	return nil
}
