package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ryotarai/arca/internal/db"
	"github.com/ryotarai/arca/internal/version"
)

type arcadBinaryHandler struct {
	store     *db.Store
	binaryDir string

	mu    sync.RWMutex
	cache map[string][]byte // key: "goos/goarch"
}

var allowedPlatforms = map[string]bool{
	"linux/amd64": true,
	"linux/arm64": true,
}

func newArcadBinaryHandler(store *db.Store) *arcadBinaryHandler {
	binaryDir := os.Getenv("ARCAD_BINARY_DIR")
	if binaryDir == "" {
		binaryDir = "/opt/arcad"
	}
	return &arcadBinaryHandler{
		store:     store,
		binaryDir: binaryDir,
		cache:     make(map[string][]byte),
	}
}

func (h *arcadBinaryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := machineTokenFromHeader(r.Header)
	if token == "" {
		http.Error(w, "machine token is required", http.StatusUnauthorized)
		return
	}

	_, err := h.store.GetMachineIDByMachineToken(r.Context(), token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "invalid machine token", http.StatusUnauthorized)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	goos := strings.TrimSpace(r.URL.Query().Get("os"))
	goarch := strings.TrimSpace(r.URL.Query().Get("arch"))
	if goos == "" || goarch == "" {
		http.Error(w, "os and arch query parameters are required", http.StatusBadRequest)
		return
	}

	key := goos + "/" + goarch
	if !allowedPlatforms[key] {
		http.Error(w, fmt.Sprintf("unsupported platform: %s", key), http.StatusBadRequest)
		return
	}

	data, err := h.getOrBuild(r.Context(), goos, goarch, key)
	if err != nil {
		http.Error(w, fmt.Sprintf("build failed: %v", err), http.StatusInternalServerError)
		return
	}

	h256 := sha256.Sum256(data)
	w.Header().Set("X-Checksum-SHA256", hex.EncodeToString(h256[:]))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=arcad")
	w.Write(data)
}

func (h *arcadBinaryHandler) getOrBuild(ctx context.Context, goos, goarch, key string) ([]byte, error) {
	h.mu.RLock()
	if data, ok := h.cache[key]; ok {
		h.mu.RUnlock()
		return data, nil
	}
	h.mu.RUnlock()

	h.mu.Lock()
	defer h.mu.Unlock()

	// Double-check after acquiring write lock.
	if data, ok := h.cache[key]; ok {
		return data, nil
	}

	// Try pre-built binary first, fall back to go build for local dev.
	prebuiltPath := filepath.Join(h.binaryDir, fmt.Sprintf("arcad_%s_%s", goos, goarch))
	data, err := os.ReadFile(prebuiltPath)
	if err != nil {
		data, err = buildArcadBinary(ctx, goos, goarch)
		if err != nil {
			return nil, err
		}
	}
	h.cache[key] = data
	return data, nil
}

func buildArcadBinary(ctx context.Context, goos, goarch string) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "arca-arcad-build-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	arcadPath := tmpDir + "/arcad"
	ldflags := fmt.Sprintf("-X github.com/ryotarai/arca/internal/version.Version=%s", version.Version)
	cmd := exec.CommandContext(ctx, "go", "build", "-ldflags", ldflags, "-o", arcadPath, "./cmd/arcad")
	cmd.Env = append(os.Environ(),
		"GOOS="+goos,
		"GOARCH="+goarch,
		"CGO_ENABLED=0",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go build ./cmd/arcad failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return os.ReadFile(arcadPath)
}
