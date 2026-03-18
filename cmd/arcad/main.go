package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ryotarai/arca/internal/arcad"
)

func main() {
	userMode := flag.Bool("user", false, "run in user mode (reverse proxy, no setup)")
	setupOnce := flag.Bool("setup-once", false, "run setup steps once and exit (for Packer image builds)")
	flag.Parse()

	// --setup-once can also be triggered via ARCAD_SETUP_ONCE=true
	if !*setupOnce && strings.EqualFold(os.Getenv("ARCAD_SETUP_ONCE"), "true") {
		*setupOnce = true
	}

	if *setupOnce {
		runSetupOnce()
		return
	}

	cfg, err := arcad.ConfigFromEnv()
	if err != nil {
		log.Fatalf("invalid config: %v", err)
	}

	// If not explicitly user mode but not running as root, fall back to user
	// mode for backward compatibility with old service files that run as
	// User=arcad.
	if !*userMode && os.Getuid() != 0 {
		log.Printf("not running as root, falling back to user mode")
		*userMode = true
	}

	if *userMode {
		runUserMode(cfg)
	} else {
		runRootMode(cfg)
	}
}

// runUserMode runs the reverse proxy and readiness reporter (existing behavior).
func runUserMode(cfg arcad.Config) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	upstreamURL, err := url.Parse(cfg.UpstreamURL)
	if err != nil {
		log.Fatalf("invalid ARCAD_UPSTREAM_URL: %v", err)
	}

	controlPlaneClient := arcad.NewHTTPControlPlaneClient(cfg.ControlPlaneURL, cfg.AuthorizeURL, cfg.MachineID, cfg.MachineToken, &http.Client{Timeout: 10 * time.Second})
	exposureCache := arcad.NewExposureCache(controlPlaneClient)
	proxy := arcad.NewProxy(exposureCache, controlPlaneClient, cfg.SessionCookie, upstreamURL, cfg.TTydSocket)
	readinessChecker := arcad.NewReadinessChecker(cfg.StartupSentinel, splitCSV(cfg.ReadyEndpoints))
	proxy.SetReadinessChecker(readinessChecker)

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           proxy,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 2)
	go func() {
		log.Printf("arcad (user) listening on %s", cfg.ListenAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	if cfg.TunnelToken != "" {
		runner := &arcad.CloudflaredRunner{
			TunnelToken:   cfg.TunnelToken,
			RestartOnExit: true,
			Stdout:        os.Stdout,
			Stderr:        os.Stderr,
		}
		go func() {
			if err := runner.Run(ctx); err != nil {
				errCh <- err
			}
		}()
	} else {
		log.Printf("ARCAD_TUNNEL_TOKEN not set; skipping cloudflared")
	}

	go arcad.NewReadinessReporter(readinessChecker, controlPlaneClient, cfg.ReadyReportInterval).Run(ctx)
	go arcad.NewLLMSyncer(controlPlaneClient, cfg.ShelleyPort, 5*time.Minute).Run(ctx)

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("arcad shutdown failed: %v", err)
		}
	case err := <-errCh:
		stop()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
		log.Fatalf("arcad failed: %v", err)
	}
}

// runRootMode runs the self-update check, idempotent setup, and then idles.
func runRootMode(cfg arcad.Config) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("arcad (root) starting")

	httpClient := &http.Client{Timeout: 60 * time.Second}

	// Self-update check (first operation).
	restarted, err := arcad.CheckAndUpdate(ctx, cfg, httpClient)
	if err != nil {
		log.Printf("self-update error: %v", err)
	}
	if restarted {
		return
	}

	// Run idempotent setup.
	setupCfg := arcad.SetupConfigFromEnv()
	if err := arcad.RunSetup(ctx, setupCfg); err != nil {
		log.Fatalf("setup failed: %v", err)
	}

	// Idle until signalled.
	log.Printf("arcad (root) setup complete, waiting for signal")
	<-ctx.Done()
}

// runSetupOnce runs the idempotent setup steps once without connecting to
// the control plane, then exits. This is intended for use inside Packer
// provisioners to pre-bake dependencies into a machine image.
func runSetupOnce() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("arcad --setup-once: running offline setup")

	setupCfg := arcad.SetupConfigFromEnv()
	if err := arcad.RunSetupOnce(ctx, setupCfg); err != nil {
		log.Fatalf("setup-once failed: %v", err)
	}
	log.Printf("arcad --setup-once: complete")
}

func splitCSV(value string) []string {
	items := strings.Split(value, ",")
	out := make([]string, 0, len(items))
	for _, item := range items {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
