package machine

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/ryotarai/arca/internal/db"
)

func TestCloudInitUserData_IncludesAndExecutesStartupScript(t *testing.T) {
	t.Parallel()

	cloudInit := cloudInitUserData(db.Machine{ID: "machine-123456789abc"}, RuntimeStartOptions{
		StartupScript:         "echo hello startup",
		InteractiveSSHPubKeys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOJ9vZxA2v4n5hF8B07A2fkYg6P5mK2xOb3d9HfNQh8S test@example.com"},
	}, "YXJjYWQ=")

	startupScript, ok := cloudInitFileContent(cloudInit, "/usr/local/bin/arca-user-startup.sh")
	if !ok {
		t.Fatalf("cloud-init does not include startup script file")
	}
	if !strings.Contains(startupScript, "echo hello startup") {
		t.Fatalf("startup script content is not propagated")
	}

	bootstrapScript, ok := cloudInitFileContent(cloudInit, "/usr/local/bin/arca-bootstrap.sh")
	if !ok {
		t.Fatalf("cloud-init does not include bootstrap script")
	}
	if !strings.Contains(bootstrapScript, "/usr/bin/env bash /usr/local/bin/arca-user-startup.sh") {
		t.Fatalf("bootstrap script does not execute startup script")
	}
	if !strings.Contains(bootstrapScript, "id -u \"$daemon_user\"") {
		t.Fatalf("bootstrap script does not provision daemon user")
	}
	if !strings.Contains(bootstrapScript, "id -u \"$interactive_user\"") {
		t.Fatalf("bootstrap script does not provision interactive user")
	}
	if !strings.Contains(bootstrapScript, "authorized_keys") {
		t.Fatalf("bootstrap script does not provision authorized_keys")
	}
	if !strings.Contains(cloudInit, "User=arcad") {
		t.Fatalf("arcad service must run as daemon user")
	}
	if !strings.Contains(cloudInit, "User=arcauser") {
		t.Fatalf("ttyd service must run as interactive user")
	}
	if !strings.Contains(cloudInit, "-U arcauser:arcauser") {
		t.Fatalf("ttyd auth user must be interactive user")
	}
}

func cloudInitFileContent(cloudInit string, path string) (string, bool) {
	lines := strings.Split(cloudInit, "\n")
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != fmt.Sprintf("- path: %s", path) {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			trimmed := strings.TrimSpace(lines[j])
			if strings.HasPrefix(trimmed, "- path: ") {
				return "", false
			}
			if !strings.HasPrefix(trimmed, "content: ") {
				continue
			}
			encoded := strings.TrimSpace(strings.TrimPrefix(trimmed, "content: "))
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				return "", false
			}
			return string(decoded), true
		}
	}
	return "", false
}
