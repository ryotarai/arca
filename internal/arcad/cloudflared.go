package arcad

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"time"
)

type CloudflaredRunner struct {
	BinaryPath    string
	TunnelToken   string
	RestartOnExit bool
	Stdout        io.Writer
	Stderr        io.Writer
}

func (r *CloudflaredRunner) Run(ctx context.Context) error {
	if r.TunnelToken == "" {
		return fmt.Errorf("empty tunnel token")
	}
	binary := r.BinaryPath
	if binary == "" {
		binary = "cloudflared"
	}
	restart := r.RestartOnExit
	for {
		cmd := exec.CommandContext(ctx, binary, "tunnel", "run", "--token", r.TunnelToken)
		cmd.Stdout = r.Stdout
		cmd.Stderr = r.Stderr
		log.Printf("starting cloudflared: %s", binary)
		err := cmd.Run()
		if ctx.Err() != nil {
			return nil
		}
		if err == nil {
			if !restart {
				return nil
			}
			log.Printf("cloudflared exited cleanly; restarting")
			time.Sleep(time.Second)
			continue
		}
		if errors.Is(err, context.Canceled) {
			return nil
		}
		if !restart {
			return fmt.Errorf("cloudflared exited: %w", err)
		}
		log.Printf("cloudflared exited with error: %v; restarting", err)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(2 * time.Second):
		}
	}
}
