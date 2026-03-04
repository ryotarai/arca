package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/ryotarai/arca/internal/cloudflare"
	"github.com/ryotarai/arca/internal/db"
)

const consoleTunnelName = "arca-console"

type ConsoleTunnelManager struct {
	rootCtx     context.Context
	cfClient    *cloudflare.Client
	originURL   string
	binaryPath  string
	mu          sync.Mutex
	runnerToken string
	runnerStop  context.CancelFunc
}

func NewConsoleTunnelManager(rootCtx context.Context, cfClient *cloudflare.Client, originURL string) *ConsoleTunnelManager {
	if rootCtx == nil {
		rootCtx = context.Background()
	}
	return &ConsoleTunnelManager{
		rootCtx:    rootCtx,
		cfClient:   cfClient,
		originURL:  strings.TrimSpace(originURL),
		binaryPath: strings.TrimSpace(os.Getenv("CLOUDFLARED_BINARY")),
	}
}

func (m *ConsoleTunnelManager) EnsureExposed(ctx context.Context, setup db.SetupState) (string, error) {
	if m == nil || m.cfClient == nil {
		return "", errors.New("cloudflare client unavailable")
	}
	if strings.TrimSpace(m.originURL) == "" {
		return "", errors.New("console origin url is not configured")
	}
	token := strings.TrimSpace(setup.CloudflareAPIToken)
	zoneID := strings.TrimSpace(setup.CloudflareZoneID)
	baseDomain := strings.TrimSpace(setup.BaseDomain)
	if token == "" || zoneID == "" || baseDomain == "" {
		return "", errors.New("cloudflare setup is incomplete")
	}

	zone, err := m.cfClient.GetZone(ctx, token, zoneID)
	if err != nil {
		return "", fmt.Errorf("load cloudflare zone: %w", err)
	}
	accountID := strings.TrimSpace(zone.Account.ID)
	if accountID == "" {
		return "", errors.New("cloudflare account id is missing")
	}

	tunnel, err := m.cfClient.GetTunnelByName(ctx, token, accountID, consoleTunnelName)
	if err != nil {
		if !errors.Is(err, cloudflare.ErrTunnelNotFound) {
			return "", fmt.Errorf("lookup console tunnel: %w", err)
		}
		tunnel, err = m.cfClient.CreateTunnel(ctx, token, accountID, consoleTunnelName)
		if err != nil {
			return "", fmt.Errorf("create console tunnel: %w", err)
		}
	}

	tunnelToken, err := m.cfClient.CreateTunnelToken(ctx, token, accountID, tunnel.ID)
	if err != nil {
		return "", fmt.Errorf("create console tunnel token: %w", err)
	}

	hostname := consoleHostname(setup.DomainPrefix, baseDomain)
	target := tunnel.ID + ".cfargotunnel.com"
	if err := m.cfClient.UpsertDNSCNAME(ctx, token, zoneID, hostname, target, true); err != nil {
		return "", fmt.Errorf("upsert console cname: %w", err)
	}
	if err := m.cfClient.UpdateTunnelIngress(ctx, token, accountID, tunnel.ID, []cloudflare.IngressRule{
		{Hostname: hostname, Service: m.originURL},
	}); err != nil {
		return "", fmt.Errorf("update console tunnel ingress: %w", err)
	}

	m.ensureCloudflared(tunnelToken)
	return hostname, nil
}

func (m *ConsoleTunnelManager) ensureCloudflared(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if strings.TrimSpace(token) == "" {
		return
	}
	if m.runnerStop != nil && token == m.runnerToken {
		return
	}
	if m.runnerStop != nil {
		m.runnerStop()
		m.runnerStop = nil
	}

	runCtx, cancel := context.WithCancel(m.rootCtx)
	m.runnerStop = cancel
	m.runnerToken = token
	binary := m.binaryPath
	go runCloudflared(runCtx, binary, token)
}

func runCloudflared(ctx context.Context, binaryPath, tunnelToken string) {
	binary := strings.TrimSpace(binaryPath)
	if binary == "" {
		binary = "cloudflared"
	}
	for {
		cmd := exec.CommandContext(ctx, binary, "tunnel", "run", "--token", tunnelToken)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		log.Printf("starting console cloudflared: %s", binary)
		err := cmd.Run()
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			log.Printf("console cloudflared exited cleanly; restarting")
		} else {
			log.Printf("console cloudflared exited with error: %v; restarting", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func consoleHostname(prefix, baseDomain string) string {
	prefix = sanitizeSubdomainPart(prefix)
	label := strings.Trim(prefix+"app", "-")
	if label == "" {
		label = "app"
	}
	return label + "." + strings.TrimSpace(baseDomain)
}

func sanitizeSubdomainPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
