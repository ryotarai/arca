package arcad

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/ryotarai/arca/internal/machine"
)

// AgentGuidelineSyncer periodically fetches the assembled agent guideline from
// the control plane and writes it to well-known agent configuration files in
// the user's home directory.
type AgentGuidelineSyncer struct {
	client   ControlPlaneClient
	homeDir  string
	interval time.Duration
	lastHash string
}

// NewAgentGuidelineSyncer creates a new syncer that writes guidelines to homeDir.
func NewAgentGuidelineSyncer(client ControlPlaneClient, homeDir string, interval time.Duration) *AgentGuidelineSyncer {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &AgentGuidelineSyncer{
		client:   client,
		homeDir:  homeDir,
		interval: interval,
	}
}

// Run starts the sync loop. It performs an initial sync immediately, then
// re-syncs on the configured interval.
func (s *AgentGuidelineSyncer) Run(ctx context.Context) {
	s.syncOnce(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.syncOnce(ctx)
		}
	}
}

func (s *AgentGuidelineSyncer) syncOnce(ctx context.Context) {
	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	guideline, err := s.client.GetMachineAgentGuideline(fetchCtx)
	if err != nil {
		log.Printf("agent-guideline-syncer: fetch guideline: %v", err)
		return
	}

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(guideline)))
	if hash == s.lastHash {
		return
	}

	if err := writeGuidelineFiles(s.homeDir, guideline); err != nil {
		log.Printf("agent-guideline-syncer: write files: %v", err)
		return
	}

	s.lastHash = hash
	log.Printf("agent-guideline-syncer: synced guideline to %s", s.homeDir)
}

var guidelineTargets = []struct {
	dir  string
	file string
}{
	{".claude", "CLAUDE.md"},
	{".codex", "AGENTS.md"},
	{".gemini", "GEMINI.md"},
	{".config", "AGENTS.md"},
}

func writeGuidelineFiles(homeDir, managedSection string) error {
	for _, target := range guidelineTargets {
		dir := filepath.Join(homeDir, target.dir)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
		path := filepath.Join(dir, target.file)
		existing, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("read %s: %w", path, err)
		}
		updated := machine.ReplaceOrAppendMarkedSection(string(existing), managedSection)
		if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}
