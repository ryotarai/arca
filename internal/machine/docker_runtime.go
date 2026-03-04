package machine

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/ryotarai/arca/internal/db"
)

const machineIDLabel = "arca.machine_id"
const defaultMachineImage = "busybox:1.36"

type DockerRuntime struct {
	client *client.Client
	image  string
}

func NewDockerRuntime(imageName string) (*DockerRuntime, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	if imageName == "" {
		imageName = defaultMachineImage
	}
	return &DockerRuntime{client: cli, image: imageName}, nil
}

func (r *DockerRuntime) EnsureRunning(ctx context.Context, machine db.Machine, opts RuntimeStartOptions) (string, error) {
	containerID := machine.ContainerID
	if containerID == "" {
		foundID, err := r.findContainerByMachineID(ctx, machine.ID)
		if err != nil {
			return "", err
		}
		containerID = foundID
	}

	if containerID != "" {
		inspected, err := r.client.ContainerInspect(ctx, containerID)
		if err != nil {
			if !errdefs.IsNotFound(err) {
				return "", err
			}
			containerID = ""
		} else if inspected.State != nil && inspected.State.Running {
			return containerID, nil
		}
	}

	if containerID == "" {
		if err := r.pullImageIfNeeded(ctx); err != nil {
			return "", err
		}

		name := "arca-machine-" + machine.ID[:12]
		config := &container.Config{
			Image: r.image,
			Env: []string{
				"ARCA_TUNNEL_TOKEN=" + opts.TunnelToken,
				"ARCAD_TUNNEL_TOKEN=" + opts.TunnelToken,
				"ARCAD_CONTROL_PLANE_URL=" + opts.ControlPlaneURL,
				"ARCAD_MACHINE_ID=" + opts.MachineID,
			},
			Labels: map[string]string{
				machineIDLabel: machine.ID,
			},
		}
		if strings.TrimSpace(opts.MachineToken) != "" {
			config.Env = append(config.Env, "ARCAD_MACHINE_TOKEN="+opts.MachineToken)
		}
		if r.image == defaultMachineImage {
			config.Cmd = []string{"sh", "-c", "while true; do sleep 3600; done"}
		}
		created, err := r.client.ContainerCreate(
			ctx,
			config,
			nil,
			nil,
			nil,
			name,
		)
		if err != nil {
			return "", err
		}
		containerID = created.ID
	}

	if err := r.client.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		if errdefs.IsNotFound(err) {
			return "", err
		}
		if strings.Contains(strings.ToLower(err.Error()), "already started") {
			return containerID, nil
		}
		return "", err
	}
	return containerID, nil
}

func (r *DockerRuntime) EnsureStopped(ctx context.Context, machine db.Machine) error {
	containerID := machine.ContainerID
	if containerID == "" {
		foundID, err := r.findContainerByMachineID(ctx, machine.ID)
		if err != nil {
			return err
		}
		containerID = foundID
	}
	if containerID == "" {
		return nil
	}

	timeout := 10
	if err := r.client.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout}); err != nil {
		if !errdefs.IsNotFound(err) {
			return err
		}
	}
	if err := r.client.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true}); err != nil {
		if !errdefs.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func (r *DockerRuntime) IsRunning(ctx context.Context, machine db.Machine) (bool, string, error) {
	containerID := machine.ContainerID
	if containerID != "" {
		inspected, err := r.client.ContainerInspect(ctx, containerID)
		if err == nil {
			if inspected.State != nil && inspected.State.Running {
				return true, containerID, nil
			}
			return false, containerID, nil
		}
		if !errdefs.IsNotFound(err) {
			return false, "", err
		}
		containerID = ""
	}

	foundID, err := r.findContainerByMachineID(ctx, machine.ID)
	if err != nil {
		return false, "", err
	}
	if foundID == "" {
		return false, "", nil
	}

	inspected, err := r.client.ContainerInspect(ctx, foundID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return false, "", nil
		}
		return false, "", err
	}
	if inspected.State != nil && inspected.State.Running {
		return true, foundID, nil
	}
	return false, foundID, nil
}

func (r *DockerRuntime) WaitReady(ctx context.Context, machine db.Machine, instanceID string) error {
	containerID := firstNonEmpty(instanceID, machine.ContainerID)
	if containerID == "" {
		foundID, err := r.findContainerByMachineID(ctx, machine.ID)
		if err != nil {
			return err
		}
		containerID = foundID
	}
	if containerID == "" {
		return fmt.Errorf("container id is empty")
	}

	inspect, err := r.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return err
	}
	ip := strings.TrimSpace(inspect.NetworkSettings.IPAddress)
	if ip == "" {
		for _, netCfg := range inspect.NetworkSettings.Networks {
			if strings.TrimSpace(netCfg.IPAddress) != "" {
				ip = strings.TrimSpace(netCfg.IPAddress)
				break
			}
		}
	}
	if ip == "" {
		return fmt.Errorf("container %s has no ip address", containerID)
	}
	return waitHTTPReady(ctx, fmt.Sprintf("http://%s:21030/__arca/readyz", ip))
}

func (r *DockerRuntime) pullImageIfNeeded(ctx context.Context) error {
	_, _, err := r.client.ImageInspectWithRaw(ctx, r.image)
	if err == nil {
		return nil
	}
	if !errdefs.IsNotFound(err) {
		return err
	}

	reader, err := r.client.ImagePull(ctx, r.image, image.PullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()
	_, _ = io.Copy(io.Discard, reader)
	return nil
}

func (r *DockerRuntime) findContainerByMachineID(ctx context.Context, machineID string) (string, error) {
	query := filters.NewArgs(filters.Arg("label", machineIDLabel+"="+machineID))
	containers, err := r.client.ContainerList(ctx, container.ListOptions{All: true, Filters: query})
	if err != nil {
		return "", err
	}
	if len(containers) == 0 {
		return "", nil
	}
	return containers[0].ID, nil
}
