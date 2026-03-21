package arcad

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ryotarai/arca/internal/version"
)

const updateMarkerPath = "/run/arca/update-checked"

// CheckAndUpdate checks for arcad binary updates and performs self-update if needed.
// Returns true if the process was restarted (should not normally be reached).
func CheckAndUpdate(ctx context.Context, cfg Config, httpClient *http.Client) (bool, error) {
	if _, err := os.Stat(updateMarkerPath); err == nil {
		log.Printf("update marker exists, skipping update check")
		return false, nil
	}

	if err := os.MkdirAll(filepath.Dir(updateMarkerPath), 0755); err != nil {
		return false, fmt.Errorf("create marker dir: %w", err)
	}

	serverVersion, err := getServerArcadVersion(ctx, cfg, httpClient)
	if err != nil {
		log.Printf("failed to check server version: %v (continuing with current version)", err)
		writeUpdateMarker()
		return false, nil
	}

	localVersion := strings.TrimSpace(version.Version)
	serverVersion = strings.TrimSpace(serverVersion)
	log.Printf("arcad version check: local=%s server=%s", localVersion, serverVersion)

	needsUpdate := localVersion == "dev" || localVersion != serverVersion
	if !needsUpdate {
		log.Printf("arcad is up to date")
		writeUpdateMarker()
		return false, nil
	}

	log.Printf("arcad update available: %s -> %s", localVersion, serverVersion)

	if err := downloadAndReplaceBinary(ctx, cfg, httpClient); err != nil {
		log.Printf("failed to download update: %v (continuing with current version)", err)
		writeUpdateMarker()
		return false, nil
	}

	writeUpdateMarker()

	log.Printf("restarting arca-arcad.service to apply update")
	cmd := exec.Command("systemctl", "restart", "arca-arcad.service")
	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("systemctl restart failed: %w", err)
	}

	return true, nil
}

func getServerArcadVersion(ctx context.Context, cfg Config, httpClient *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.ControlPlaneURL+"/arcad/version", nil)
	if err != nil {
		return "", err
	}
	if cfg.MachineToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.MachineToken)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("version check returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}

func downloadAndReplaceBinary(ctx context.Context, cfg Config, httpClient *http.Client) error {
	downloadURL := fmt.Sprintf("%s/arcad/download?os=linux&arch=%s", cfg.ControlPlaneURL, runtime.GOARCH)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
	if cfg.MachineToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.MachineToken)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	binaryPath := "/usr/local/bin/arcad"
	tmpFile, err := os.CreateTemp(filepath.Dir(binaryPath), ".arcad-update-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		tmpFile.Close()
		os.Remove(tmpPath)
	}()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return fmt.Errorf("write binary: %w", err)
	}

	// Verify SHA256 checksum if the server provided one.
	expectedHash := resp.Header.Get("X-Checksum-SHA256")
	if expectedHash != "" {
		if _, err := tmpFile.Seek(0, 0); err != nil {
			return fmt.Errorf("seek temp file: %w", err)
		}
		h := sha256.New()
		if _, err := io.Copy(h, tmpFile); err != nil {
			return fmt.Errorf("compute checksum: %w", err)
		}
		actualHash := hex.EncodeToString(h.Sum(nil))
		if actualHash != expectedHash {
			return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHash, actualHash)
		}
		log.Printf("binary checksum verified: %s", actualHash)
	} else {
		log.Printf("server did not provide checksum header, skipping verification")
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}
	if err := os.Rename(tmpPath, binaryPath); err != nil {
		return fmt.Errorf("rename: %w", err)
	}

	log.Printf("arcad binary updated at %s", binaryPath)
	return nil
}

func writeUpdateMarker() {
	if err := os.MkdirAll(filepath.Dir(updateMarkerPath), 0755); err != nil {
		log.Printf("failed to create marker dir: %v", err)
		return
	}
	if err := os.WriteFile(updateMarkerPath, []byte("1"), 0644); err != nil {
		log.Printf("failed to write update marker: %v", err)
	}
}
