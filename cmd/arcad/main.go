package main

import (
	"context"
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
	cfg, err := arcad.ConfigFromEnv()
	if err != nil {
		log.Fatalf("invalid config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	upstreamURL, err := url.Parse(cfg.UpstreamURL)
	if err != nil {
		log.Fatalf("invalid ARCAD_UPSTREAM_URL: %v", err)
	}

	controlPlaneClient := arcad.NewHTTPControlPlaneClient(cfg.ControlPlaneURL, cfg.AuthorizeURL, cfg.MachineID, cfg.MachineToken, &http.Client{Timeout: 10 * time.Second})
	exposureCache := arcad.NewExposureCache(controlPlaneClient)
	sessions := arcad.NewSessionManager(cfg.MachineToken)
	proxy := arcad.NewProxy(exposureCache, controlPlaneClient, sessions, cfg.SessionCookie, upstreamURL)
	proxy.SetReadinessChecker(arcad.NewReadinessChecker(cfg.StartupSentinel, splitCSV(cfg.ReadyEndpoints)))

	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           proxy,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 2)
	go func() {
		log.Printf("arcad listening on %s", cfg.ListenAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

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
