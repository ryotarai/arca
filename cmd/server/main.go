package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/ryotarai/arca/internal/auth"
	"github.com/ryotarai/arca/internal/cloudflare"
	"github.com/ryotarai/arca/internal/db"
	"github.com/ryotarai/arca/internal/machine"
	"github.com/ryotarai/arca/internal/server"
)

func main() {
	addr := os.Getenv("SERVER_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	dbConfig := db.ConfigFromEnv()
	sqlDB, err := db.Open(dbConfig)
	if err != nil {
		log.Fatalf("db open failed: %v", err)
	}
	store := db.NewStore(sqlDB, dbConfig.Driver)
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("db close failed: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := db.ApplyMigrations(ctx, sqlDB); err != nil {
		log.Fatalf("db migration failed: %v", err)
	}
	authService := auth.NewService(store)
	cfClient := cloudflare.NewClient(http.DefaultClient)
	dockerRuntime, err := machine.NewDockerRuntime(os.Getenv("MACHINE_DOCKER_IMAGE"))
	if err != nil {
		log.Fatalf("docker runtime initialization failed: %v", err)
	}
	machineWorker := machine.NewWorker(store, dockerRuntime, cfClient, "worker-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	go machineWorker.Run(ctx)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           server.NewRouter(server.Dependencies{HealthChecker: store, Authenticator: authService, MachineStore: store, Store: store, Cloudflare: cfClient}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("server listening on %s", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Fatalf("graceful shutdown failed: %v", err)
		}
		log.Println("server stopped")
	case err := <-errCh:
		log.Fatalf("server failed: %v", err)
	}
}
