package machine

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/ryotarai/arca/internal/db"
)

const (
	defaultLxdEndpoint    = "https://localhost:8443"
	defaultLxdArcadGOOS = "linux"
	defaultLxdImage       = "ubuntu:24.04"
)

type LxdRuntime struct {
	endpoint      string
	startupScript string
	arcadGOOS     string
	arcadGOARCH   string
	image         string
}

type LxdRuntimeOptions struct {
	Endpoint      string
	StartupScript string
	ArcadGOOS     string
	ArcadGOARCH   string
	Image         string
}

func NewLxdRuntimeWithOptions(options LxdRuntimeOptions) *LxdRuntime {
	endpoint := strings.TrimSpace(options.Endpoint)
	if endpoint == "" {
		endpoint = strings.TrimSpace(os.Getenv("ARCA_LXD_ENDPOINT"))
	}
	if endpoint == "" {
		endpoint = defaultLxdEndpoint
	}

	startupScript := options.StartupScript
	if strings.TrimSpace(startupScript) == "" {
		startupScript = ""
	}

	arcadGOOS := strings.TrimSpace(options.ArcadGOOS)
	if arcadGOOS == "" {
		arcadGOOS = strings.TrimSpace(os.Getenv("ARCA_LXD_ARCAD_GOOS"))
	}
	if arcadGOOS == "" {
		arcadGOOS = defaultLxdArcadGOOS
	}

	arcadGOARCH := strings.TrimSpace(options.ArcadGOARCH)
	if arcadGOARCH == "" {
		arcadGOARCH = strings.TrimSpace(os.Getenv("ARCA_LXD_ARCAD_GOARCH"))
	}
	if arcadGOARCH == "" {
		arcadGOARCH = runtime.GOARCH
	}

	image := strings.TrimSpace(options.Image)
	if image == "" {
		image = strings.TrimSpace(os.Getenv("ARCA_LXD_IMAGE"))
	}
	if image == "" {
		image = defaultLxdImage
	}

	return &LxdRuntime{
		endpoint:      endpoint,
		startupScript: startupScript,
		arcadGOOS:     arcadGOOS,
		arcadGOARCH:   arcadGOARCH,
		image:         image,
	}
}

func (r *LxdRuntime) EnsureRunning(ctx context.Context, machine db.Machine, opts RuntimeStartOptions) (string, error) {
	containerName := r.containerName(machine)

	exists, err := r.containerExists(ctx, containerName)
	if err != nil {
		return "", err
	}

	if !exists {
		arcadBinaryBase64, err := r.buildArcadBinaryBase64(ctx)
		if err != nil {
			return "", err
		}

		opts.StartupScript = r.startupScript
		cloudConfig := cloudInitUserData(machine, opts, arcadBinaryBase64)

		if err := r.launchContainer(ctx, containerName, cloudConfig); err != nil {
			return "", err
		}
		return containerName, nil
	}

	running, _, err := r.IsRunning(ctx, machine)
	if err != nil {
		return "", err
	}
	if running {
		return containerName, nil
	}

	if _, err := r.runLxc(ctx, "start", containerName); err != nil {
		return "", err
	}
	return containerName, nil
}

func (r *LxdRuntime) EnsureStopped(ctx context.Context, machine db.Machine) error {
	containerName := r.containerName(machine)

	exists, err := r.containerExists(ctx, containerName)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	running, _, err := r.IsRunning(ctx, machine)
	if err != nil {
		return err
	}
	if !running {
		return nil
	}

	_, _ = r.runLxc(ctx, "stop", containerName, "--timeout", "30")
	deadline := time.Now().Add(35 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		running, _, err = r.IsRunning(ctx, machine)
		if err != nil {
			return err
		}
		if !running {
			return nil
		}
	}

	if _, err := r.runLxc(ctx, "stop", containerName, "--force"); err != nil {
		return err
	}
	return nil
}

func (r *LxdRuntime) EnsureDeleted(ctx context.Context, machine db.Machine) error {
	if err := r.EnsureStopped(ctx, machine); err != nil {
		return err
	}

	containerName := r.containerName(machine)
	exists, err := r.containerExists(ctx, containerName)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	if _, err := r.runLxc(ctx, "delete", containerName, "--force"); err != nil {
		return err
	}
	return nil
}

func (r *LxdRuntime) IsRunning(ctx context.Context, machine db.Machine) (bool, string, error) {
	containerName := r.containerName(machine)
	output, err := r.runLxc(ctx, "info", containerName)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return false, "", nil
		}
		return false, "", err
	}
	state := strings.ToLower(output)
	if strings.Contains(state, "status: running") {
		return true, containerName, nil
	}
	return false, containerName, nil
}

func (r *LxdRuntime) containerName(machine db.Machine) string {
	if strings.TrimSpace(machine.ContainerID) != "" {
		return machine.ContainerID
	}
	return "arca-machine-" + machine.ID[:12]
}

func (r *LxdRuntime) containerExists(ctx context.Context, name string) (bool, error) {
	_, err := r.runLxc(ctx, "info", name)
	if err == nil {
		return true, nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "not found") {
		return false, nil
	}
	return false, err
}

func (r *LxdRuntime) launchContainer(ctx context.Context, name, cloudConfig string) error {
	// Use lxc init + config set + start instead of lxc launch --config
	// to avoid "argument list too long" when cloud-init data (which includes
	// the arcad binary) exceeds the OS command-line argument limit.
	if _, err := r.runLxc(ctx, "init", r.image, name); err != nil {
		return fmt.Errorf("init lxd container: %w", err)
	}

	// Pass user-data via stdin to avoid argument length limits.
	cmd := exec.CommandContext(ctx, "lxc", "config", "set", name, "user.user-data", "-")
	cmd.Stdin = strings.NewReader(cloudConfig)
	if output, err := cmd.CombinedOutput(); err != nil {
		// Clean up the created-but-not-started container on failure.
		_, _ = r.runLxc(ctx, "delete", name, "--force")
		return fmt.Errorf("set cloud-init config: %w: %s", err, strings.TrimSpace(string(output)))
	}

	if _, err := r.runLxc(ctx, "start", name); err != nil {
		_, _ = r.runLxc(ctx, "delete", name, "--force")
		return fmt.Errorf("start lxd container: %w", err)
	}
	return nil
}

func (r *LxdRuntime) buildArcadBinaryBase64(ctx context.Context) (string, error) {
	tmpDir, err := os.MkdirTemp("", "arca-lxd-build-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	arcadPath := tmpDir + "/arcad"
	cmd := exec.CommandContext(ctx, "go", "build", "-o", arcadPath, "./cmd/arcad")
	cmd.Env = append(os.Environ(),
		"GOOS="+r.arcadGOOS,
		"GOARCH="+r.arcadGOARCH,
		"CGO_ENABLED=0",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go build ./cmd/arcad failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	data, err := os.ReadFile(arcadPath)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func (r *LxdRuntime) runLxc(ctx context.Context, args ...string) (string, error) {
	return runCommand(ctx, "lxc", args...)
}
