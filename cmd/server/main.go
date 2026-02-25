package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ryotarai/hayai/internal/db"
	"github.com/ryotarai/hayai/internal/server"
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

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           server.NewRouter(server.Dependencies{HealthChecker: store}),
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
