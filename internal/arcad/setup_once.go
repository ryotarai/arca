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
func RunSetupOnce(ctx context.Context, _ SetupConfig) error {
	log.Printf("setup-once: ensuring ansible is installed")
	if err := ensureAnsible(ctx); err != nil {
		return fmt.Errorf("ensure ansible: %w", err)
	}

	log.Printf("setup-once: running ansible playbook (skip runtime-only roles)")
	if err := extractAndRunPlaybook(ctx,
		"runtime_only",
	); err != nil {
		return fmt.Errorf("ansible playbook: %w", err)
	}

	log.Printf("setup-once: complete")
	return nil
}
