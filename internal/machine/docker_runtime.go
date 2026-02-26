package machine

import (
	"context"
	"io"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/ryotarai/hayai/internal/db"
)

const machineIDLabel = "hayai.machine_id"

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
		imageName = "busybox:1.36"
	}
	return &DockerRuntime{client: cli, image: imageName}, nil
}

func (r *DockerRuntime) EnsureRunning(ctx context.Context, machine db.Machine) (string, error) {
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

		name := "hayai-machine-" + machine.ID[:12]
		created, err := r.client.ContainerCreate(
			ctx,
			&container.Config{
				Image: r.image,
				Cmd:   []string{"sh", "-c", "while true; do sleep 3600; done"},
				Labels: map[string]string{
					machineIDLabel: machine.ID,
				},
			},
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
