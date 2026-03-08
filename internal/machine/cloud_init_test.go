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
		StartupScript: "echo hello startup",
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
