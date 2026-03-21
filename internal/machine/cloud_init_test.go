package machine

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/ryotarai/arca/internal/db"
)

func TestCloudInitUserData_MinimalStructure(t *testing.T) {
	t.Parallel()

	cloudInit := cloudInitUserData(db.Machine{ID: "machine-123456789abc"}, RuntimeStartOptions{
		StartupScript:   "echo hello startup",
		ControlPlaneURL: "https://arca.example.com",
		MachineToken:    "test-token-123",
	})

	// User startup script is included.
	startupScript, ok := cloudInitFileContent(cloudInit, "/usr/local/bin/arca-user-startup.sh")
	if !ok {
		t.Fatalf("cloud-init does not include startup script file")
	}
	if !strings.Contains(startupScript, "echo hello startup") {
		t.Fatalf("startup script content is not propagated")
	}

	// Install script downloads arcad binary.
	installScript, ok := cloudInitFileContent(cloudInit, "/usr/local/bin/arca-install.sh")
	if !ok {
		t.Fatalf("cloud-init does not include install script")
	}
	if !strings.Contains(installScript, "/arcad/download?os=linux&arch=${goarch}") {
		t.Fatalf("install script does not download arcad binary")
	}
	if !strings.Contains(installScript, "systemctl enable --now arca-arcad.service") {
		t.Fatalf("install script does not start arcad service")
	}

	// arcad root service runs without User= (as root).
	if !strings.Contains(cloudInit, "arca-arcad.service") {
		t.Fatalf("cloud-init does not include arcad service file")
	}
	if !strings.Contains(cloudInit, "ExecStart=/usr/local/bin/arcad") {
		t.Fatalf("arcad service does not start arcad binary")
	}

	// Env file includes required variables.
	envFile, ok := cloudInitFileContent(cloudInit, "/etc/arca/arcad.env")
	if !ok {
		t.Fatalf("cloud-init does not include env file")
	}
	for _, key := range []string{
		"ARCAD_CONTROL_PLANE_URL=",
		"ARCAD_MACHINE_TOKEN=",
		"ARCA_DAEMON_USER=",
		"ARCA_INTERACTIVE_USER=",
	} {
		if !strings.Contains(envFile, key) {
			t.Fatalf("env file missing %s", key)
		}
	}

	// Cloud-init should NOT include old bootstrap or provisioning scripts.
	if _, ok := cloudInitFileContent(cloudInit, "/usr/local/bin/arca-bootstrap.sh"); ok {
		t.Fatalf("cloud-init should not include old bootstrap script (setup is in arcad)")
	}
	if _, ok := cloudInitFileContent(cloudInit, "/usr/local/bin/arca-machine-install.sh"); ok {
		t.Fatalf("cloud-init should not include old install script")
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
