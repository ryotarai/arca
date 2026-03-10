package machine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ryotarai/arca/internal/db"
)

const (
	defaultLibvirtWorkspaceDir = "/var/lib/arca/libvirt"
	defaultLibvirtBaseImage    = "/var/lib/libvirt/images/ubuntu-24.04-server-cloudimg-amd64.img"
	defaultLibvirtDiskSize     = "40G"
	defaultLibvirtURI          = "qemu:///system"
	defaultLibvirtNetwork      = "default"
	defaultLibvirtStoragePool  = "default"
)

type LibvirtRuntime struct {
	workspaceDir  string
	baseImage     string
	diskSize      string
	uri           string
	network       string
	storagePool   string
	startupScript string
}

type LibvirtRuntimeOptions struct {
	WorkspaceDir  string
	BaseImage     string
	DiskSize      string
	URI           string
	Network       string
	StoragePool   string
	StartupScript string
}

func NewLibvirtRuntime() *LibvirtRuntime {
	return NewLibvirtRuntimeWithOptions(LibvirtRuntimeOptions{})
}

func NewLibvirtRuntimeWithOptions(options LibvirtRuntimeOptions) *LibvirtRuntime {
	workspaceDir := strings.TrimSpace(options.WorkspaceDir)
	if workspaceDir == "" {
		workspaceDir = strings.TrimSpace(os.Getenv("ARCA_LIBVIRT_WORKSPACE_DIR"))
	}
	if workspaceDir == "" {
		workspaceDir = defaultLibvirtWorkspaceDir
	}

	baseImage := strings.TrimSpace(options.BaseImage)
	if baseImage == "" {
		baseImage = strings.TrimSpace(os.Getenv("ARCA_LIBVIRT_BASE_IMAGE"))
	}
	if baseImage == "" {
		baseImage = defaultLibvirtBaseImage
	}

	diskSize := strings.TrimSpace(options.DiskSize)
	if diskSize == "" {
		diskSize = strings.TrimSpace(os.Getenv("ARCA_LIBVIRT_DISK_SIZE"))
	}
	if diskSize == "" {
		diskSize = defaultLibvirtDiskSize
	}

	uri := strings.TrimSpace(options.URI)
	if uri == "" {
		uri = strings.TrimSpace(os.Getenv("ARCA_LIBVIRT_URI"))
	}
	if uri == "" {
		uri = defaultLibvirtURI
	}

	network := strings.TrimSpace(options.Network)
	if network == "" {
		network = defaultLibvirtNetwork
	}

	storagePool := strings.TrimSpace(options.StoragePool)
	if storagePool == "" {
		storagePool = defaultLibvirtStoragePool
	}
	startupScript := options.StartupScript
	if strings.TrimSpace(startupScript) == "" {
		startupScript = ""
	}

	return &LibvirtRuntime{
		workspaceDir:  workspaceDir,
		baseImage:     baseImage,
		diskSize:      diskSize,
		uri:           uri,
		network:       network,
		storagePool:   storagePool,
		startupScript: startupScript,
	}
}

func (r *LibvirtRuntime) EnsureRunning(ctx context.Context, machine db.Machine, opts RuntimeStartOptions) (string, error) {
	if _, err := os.Stat(r.baseImage); err != nil {
		return "", fmt.Errorf("libvirt base image %q is not available: %w", r.baseImage, err)
	}

	domainName := r.domainName(machine)
	workspace := filepath.Join(r.workspaceDir, machine.ID)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return "", err
	}

	if err := r.ensureDiskImage(ctx, workspace); err != nil {
		return "", err
	}
	opts.StartupScript = r.startupScript
	startupNonce := time.Now().UTC().Format("20060102T150405")
	if err := r.ensureCloudInitSeed(ctx, machine, workspace, opts, startupNonce); err != nil {
		return "", err
	}

	defined, err := r.isDomainDefined(ctx, domainName)
	if err != nil {
		return "", err
	}
	if !defined {
		if err := os.WriteFile(filepath.Join(workspace, "domain.xml"), []byte(r.domainXML(domainName, workspace)), 0o644); err != nil {
			return "", err
		}
		if _, err := r.runVirsh(ctx, "define", filepath.Join(workspace, "domain.xml")); err != nil {
			return "", err
		}
	}

	running, _, err := r.IsRunning(ctx, machine)
	if err != nil {
		return "", err
	}
	if running {
		return domainName, nil
	}

	if _, err := r.runVirsh(ctx, "start", domainName); err != nil {
		return "", err
	}
	return domainName, nil
}

func (r *LibvirtRuntime) EnsureStopped(ctx context.Context, machine db.Machine) error {
	domainName := r.domainName(machine)
	defined, err := r.isDomainDefined(ctx, domainName)
	if err != nil {
		return err
	}
	if !defined {
		return nil
	}

	running, _, err := r.IsRunning(ctx, machine)
	if err != nil {
		return err
	}
	if running {
		_, _ = r.runVirsh(ctx, "shutdown", domainName)
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(2 * time.Second)
			running, _, err = r.IsRunning(ctx, machine)
			if err != nil {
				return err
			}
			if !running {
				break
			}
		}
		if running {
			if _, err := r.runVirsh(ctx, "destroy", domainName); err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *LibvirtRuntime) EnsureDeleted(ctx context.Context, machine db.Machine) error {
	if err := r.EnsureStopped(ctx, machine); err != nil {
		return err
	}

	domainName := r.domainName(machine)
	defined, err := r.isDomainDefined(ctx, domainName)
	if err != nil {
		return err
	}
	if defined {
		if err := r.undefineDomain(ctx, domainName); err != nil {
			return err
		}
	}

	workspace := filepath.Join(r.workspaceDir, machine.ID)
	if err := os.RemoveAll(workspace); err != nil {
		return err
	}
	return nil
}

func (r *LibvirtRuntime) IsRunning(ctx context.Context, machine db.Machine) (bool, string, error) {
	domainName := r.domainName(machine)
	output, err := r.runVirsh(ctx, "domstate", domainName)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "failed to get domain") {
			return false, "", nil
		}
		return false, "", err
	}
	state := strings.ToLower(strings.TrimSpace(output))
	if strings.Contains(state, "running") || strings.Contains(state, "in shutdown") {
		return true, domainName, nil
	}
	return false, domainName, nil
}

func (r *LibvirtRuntime) GetMachineInfo(ctx context.Context, machine db.Machine) (*RuntimeMachineInfo, error) {
	domainName := r.domainName(machine)
	output, err := r.runVirsh(ctx, "domifaddr", domainName)
	if err != nil {
		return nil, fmt.Errorf("get libvirt domain addresses for %q: %w", domainName, err)
	}
	info := &RuntimeMachineInfo{}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		addr := fields[3]
		if idx := strings.Index(addr, "/"); idx > 0 {
			addr = addr[:idx]
		}
		if strings.TrimSpace(addr) != "" {
			info.PrivateIP = strings.TrimSpace(addr)
			break
		}
	}
	return info, nil
}

func (r *LibvirtRuntime) domainName(machine db.Machine) string {
	if strings.TrimSpace(machine.ContainerID) != "" {
		return machine.ContainerID
	}
	return "arca-machine-" + machine.ID[:12]
}

func (r *LibvirtRuntime) ensureDiskImage(ctx context.Context, workspace string) error {
	diskPath := filepath.Join(workspace, "disk.qcow2")
	if _, err := os.Stat(diskPath); err == nil {
		return nil
	}
	_, err := runCommand(ctx, "qemu-img", "create", "-f", "qcow2", "-F", "qcow2", "-b", r.baseImage, diskPath, r.diskSize)
	return err
}

func (r *LibvirtRuntime) ensureCloudInitSeed(ctx context.Context, machine db.Machine, workspace string, opts RuntimeStartOptions, startupNonce string) error {
	userDataPath := filepath.Join(workspace, "user-data")
	metaDataPath := filepath.Join(workspace, "meta-data")
	seedPath := filepath.Join(workspace, "seed.iso")

	userData := cloudInitUserData(machine, opts)
	metaData := fmt.Sprintf("instance-id: %s-%s\nlocal-hostname: arca-%s\n", machine.ID, startupNonce, machine.ID[:12])

	if err := os.WriteFile(userDataPath, []byte(userData), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(metaDataPath, []byte(metaData), 0o644); err != nil {
		return err
	}
	_, err := runCommand(ctx, "cloud-localds", seedPath, userDataPath, metaDataPath)
	return err
}

func (r *LibvirtRuntime) isDomainDefined(ctx context.Context, domainName string) (bool, error) {
	_, err := r.runVirsh(ctx, "dominfo", domainName)
	if err == nil {
		return true, nil
	}
	if isLibvirtDomainNotFoundError(err) {
		return false, nil
	}
	return false, err
}

func (r *LibvirtRuntime) undefineDomain(ctx context.Context, domainName string) error {
	commands := [][]string{
		{"undefine", domainName, "--managed-save", "--snapshots-metadata", "--checkpoints-metadata", "--nvram"},
		{"undefine", domainName, "--managed-save", "--snapshots-metadata", "--checkpoints-metadata"},
		{"undefine", domainName},
	}

	var lastErr error
	for _, args := range commands {
		if _, err := r.runVirsh(ctx, args...); err != nil {
			if isLibvirtDomainNotFoundError(err) {
				return nil
			}
			lastErr = err
			continue
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return nil
}

func isLibvirtDomainNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "failed to get domain") || strings.Contains(msg, "domain not found")
}

func (r *LibvirtRuntime) domainXML(domainName, workspace string) string {
	diskPath := filepath.Join(workspace, "disk.qcow2")
	seedPath := filepath.Join(workspace, "seed.iso")
	return fmt.Sprintf(`<domain type='%s'>
  <name>%s</name>
  <memory unit='MiB'>4096</memory>
  <vcpu>2</vcpu>
  <os>
    <type arch='x86_64' machine='pc-q35-7.2'>hvm</type>
    <boot dev='hd'/>
  </os>
  <features>
    <acpi/>
    <apic/>
  </features>
  <cpu mode='host-model'/>
  <devices>
    <disk type='file' device='disk'>
      <driver name='qemu' type='qcow2'/>
      <source file='%s'/>
      <target dev='vda' bus='virtio'/>
    </disk>
    <disk type='file' device='cdrom'>
      <driver name='qemu' type='raw'/>
      <source file='%s'/>
      <target dev='sda' bus='sata'/>
      <readonly/>
    </disk>
    <interface type='network'>
      <source network='%s'/>
      <model type='virtio'/>
    </interface>
    <console type='pty'/>
    <serial type='pty'/>
  </devices>
</domain>
`, r.domainType(), domainName, diskPath, seedPath, r.network)
}

func (r *LibvirtRuntime) domainType() string {
	if _, err := os.Stat("/dev/kvm"); err == nil {
		return "kvm"
	}
	return "qemu"
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s failed: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func (r *LibvirtRuntime) runVirsh(ctx context.Context, args ...string) (string, error) {
	base := []string{"-c", r.uri}
	base = append(base, args...)
	return runCommand(ctx, "virsh", base...)
}

