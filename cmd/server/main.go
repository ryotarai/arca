package main

import (
	"context"
	"crypto/rand"
	"log"
	"log/slog"
	"math/big"
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
	"github.com/ryotarai/arca/internal/crypto"
	"github.com/ryotarai/arca/internal/db"
	"github.com/ryotarai/arca/internal/machine"
	"github.com/ryotarai/arca/internal/notification"
	"github.com/ryotarai/arca/internal/server"
)

func main() {
	var logLevel slog.Level
	if err := logLevel.UnmarshalText([]byte(os.Getenv("LOG_LEVEL"))); err != nil {
		logLevel = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))

	addr := os.Getenv("SERVER_ADDR")
	if addr == "" {
		if port := os.Getenv("PORT"); port != "" {
			addr = ":" + port
		} else {
			addr = ":8080"
		}
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
	if err := ensureSetupPassword(ctx, store); err != nil {
		log.Fatalf("setup password init failed: %v", err)
	}
	authService := auth.NewService(store)
	if apiToken := os.Getenv("ARCA_API_TOKEN"); apiToken != "" {
		authService.SetStaticAPIToken(apiToken)
		log.Printf("static API token enabled")
	}
	cfClient := cloudflare.NewClient(http.DefaultClient)
	consoleTunnel := server.NewConsoleTunnelManager(ctx, cfClient, consoleOriginURL(addr))
	if setupState, setupErr := store.GetSetupState(ctx); setupErr != nil {
		log.Printf("load setup state for console tunnel failed: %v", setupErr)
	} else if setupState.Completed && setupState.ServerExposureMethod == db.ServerExposureMethodCloudflareTunnel {
		if hostname, ensureErr := consoleTunnel.EnsureExposed(ctx, setupState); ensureErr != nil {
			log.Printf("ensure console tunnel failed: %v", ensureErr)
		} else {
			log.Printf("console endpoint exposed at https://%s", hostname)
		}
	}
	workerConcurrency := 4
	if v := os.Getenv("ARCA_WORKER_CONCURRENCY"); v != "" {
		if n, parseErr := strconv.Atoi(v); parseErr == nil && n > 0 {
			workerConcurrency = n
		}
	}
	runtime := machine.NewRoutingRuntimeWithCatalog(store, map[string]machine.Runtime{})
	ipCache := machine.NewMachineIPCache(runtime, store, 5*time.Minute)
	slackService := notification.NewSlackService(store)
	machineWorker := machine.NewWorker(store, runtime, cfClient, "worker-"+strconv.FormatInt(time.Now().UnixNano(), 10), ipCache, workerConcurrency)
	machineWorker.SetNotifier(slackService)
	workerDone := make(chan struct{})
	go func() {
		machineWorker.Run(ctx)
		close(workerDone)
	}()

	var encryptor *crypto.Encryptor
	if encKey := os.Getenv("ARCA_ENCRYPTION_KEY"); encKey != "" {
		var encErr error
		encryptor, encErr = crypto.NewEncryptor(encKey)
		if encErr != nil {
			log.Fatalf("invalid ARCA_ENCRYPTION_KEY: %v", encErr)
		}
		log.Printf("API key encryption enabled")
	}

	machineProxy := server.NewMachineProxyHandler(store, ipCache)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           server.NewRouter(server.Dependencies{HealthChecker: store, Authenticator: authService, MachineStore: store, Store: store, Cloudflare: cfClient, ConsoleTunnel: consoleTunnel, MachineProxy: machineProxy, Slack: slackService, Encryptor: encryptor}),
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
		<-workerDone
		log.Println("server stopped")
	case err := <-errCh:
		log.Fatalf("server failed: %v", err)
	}
}

func ensureSetupPassword(ctx context.Context, store *db.Store) error {
	state, err := store.GetSetupState(ctx)
	if err != nil {
		return err
	}
	if state.Completed {
		return nil
	}
	existing, err := store.GetSetupPassword(ctx)
	if err != nil {
		return err
	}
	if existing != "" {
		log.Printf("Setup password: %s", existing)
		return nil
	}
	pw, err := generateSetupPassword(16)
	if err != nil {
		return err
	}
	if err := store.SetSetupPassword(ctx, pw); err != nil {
		return err
	}
	log.Printf("Setup password: %s", pw)
	return nil
}

func generateSetupPassword(length int) (string, error) {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[n.Int64()]
	}
	return string(b), nil
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
