package machine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ryotarai/arca/internal/db"
)

const (
	defaultLxdEndpoint    = "https://localhost:8443"
	defaultLxdImage       = "ubuntu:24.04"
)

type LxdRuntime struct {
	endpoint      string
	startupScript string
	image         string
}

type LxdRuntimeOptions struct {
	Endpoint      string
	StartupScript string
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
		// Backward compatibility: for pre-migration machines whose infrastructure
		// config still contains startup_script, fall back to the runtime's baked-in
		// script. For new machines, startup_script is always passed via opts from
		// the live profile.
		if opts.StartupScript == "" {
			opts.StartupScript = r.startupScript
		}
		cloudConfig := cloudInitUserData(machine, opts)
		image := r.resolveImage(machine)

		if err := r.launchContainer(ctx, containerName, image, cloudConfig); err != nil {
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

func (r *LxdRuntime) GetMachineInfo(ctx context.Context, machine db.Machine) (*RuntimeMachineInfo, error) {
	containerName := r.containerName(machine)
	output, err := r.runLxc(ctx, "list", containerName, "--format=csv", "--columns=4")
	if err != nil {
		return nil, fmt.Errorf("get lxd container addresses for %q: %w", containerName, err)
	}
	info := &RuntimeMachineInfo{}
	for _, part := range strings.Split(strings.TrimSpace(output), "\n") {
		for _, addr := range strings.Split(part, ",") {
			addr = strings.TrimSpace(addr)
			// lxc list outputs addresses as "IP (IFACE)"
			if idx := strings.Index(addr, " "); idx > 0 {
				addr = addr[:idx]
			}
			if addr != "" && !strings.Contains(addr, ":") && info.PrivateIP == "" {
				info.PrivateIP = addr
			}
		}
	}
	return info, nil
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

func (r *LxdRuntime) resolveImage(machine db.Machine) string {
	opts := parseMachineOptionsMap(machine)
	if opts != nil {
		if alias := strings.TrimSpace(opts["custom_image_image_alias"]); alias != "" {
			return alias
		}
		if fp := strings.TrimSpace(opts["custom_image_image_fingerprint"]); fp != "" {
			return fp
		}
	}
	return r.image
}

func (r *LxdRuntime) launchContainer(ctx context.Context, name, image, cloudConfig string) error {
	// Use lxc init + config set + start instead of lxc launch --config
	// to avoid "argument list too long" when cloud-init data (which includes
	// the arcad binary) exceeds the OS command-line argument limit.
	if _, err := r.runLxc(ctx, "init", image, name); err != nil {
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

func (r *LxdRuntime) runLxc(ctx context.Context, args ...string) (string, error) {
	return runCommand(ctx, "lxc", args...)
}
