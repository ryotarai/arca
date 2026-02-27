package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
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

	controlPlaneClient := arcad.NewHTTPControlPlaneClient(cfg.ControlPlaneURL, cfg.MachineID, cfg.MachineToken, &http.Client{Timeout: 10 * time.Second})
	exposureCache := arcad.NewExposureCache(controlPlaneClient)
	sessions := arcad.NewSessionManager(cfg.MachineToken)
	proxy := arcad.NewProxy(exposureCache, controlPlaneClient, sessions, cfg.SessionCookie)

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
