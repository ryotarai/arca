package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strconv"
	"strings"
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

	if err := db.ApplyMigrations(ctx, sqlDB, dbConfig.Driver); err != nil {
		log.Fatalf("db migration failed: %v", err)
	}
	authService := auth.NewService(store)
	cfClient := cloudflare.NewClient(http.DefaultClient)
	consoleTunnel := server.NewConsoleTunnelManager(ctx, cfClient, consoleOriginURL(addr))
	if setupState, setupErr := store.GetSetupState(ctx); setupErr != nil {
		log.Printf("load setup state for console tunnel failed: %v", setupErr)
	} else if setupState.Completed {
		if hostname, ensureErr := consoleTunnel.EnsureExposed(ctx, setupState); ensureErr != nil {
			log.Printf("ensure console tunnel failed: %v", ensureErr)
		} else {
			log.Printf("console endpoint exposed at https://%s", hostname)
		}
	}
	dockerRuntime, err := machine.NewDockerRuntime(os.Getenv("MACHINE_DOCKER_IMAGE"))
	if err != nil {
		log.Fatalf("docker runtime initialization failed: %v", err)
	}
	machineWorker := machine.NewWorker(store, dockerRuntime, cfClient, "worker-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	go machineWorker.Run(ctx)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           server.NewRouter(server.Dependencies{HealthChecker: store, Authenticator: authService, MachineStore: store, Store: store, Cloudflare: cfClient, ConsoleTunnel: consoleTunnel}),
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

func consoleOriginURL(listenAddr string) string {
	addr := strings.TrimSpace(listenAddr)
	if addr == "" {
		return "http://127.0.0.1:8080"
	}
	hostPort := addr
	if !strings.Contains(hostPort, ":") {
		hostPort = hostPort + ":8080"
	}
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		return "http://127.0.0.1:8080"
	}
	host = strings.TrimSpace(host)
	port = strings.TrimSpace(port)
	if port == "" {
		port = "8080"
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	} else if ip, parseErr := netip.ParseAddr(host); parseErr == nil && ip.IsUnspecified() {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}
