package machine

import (
	"context"
	"encoding/base64"
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
)

type LibvirtRuntime struct {
	workspaceDir string
	baseImage    string
	diskSize     string
}

func NewLibvirtRuntime() *LibvirtRuntime {
	workspaceDir := strings.TrimSpace(os.Getenv("ARCA_LIBVIRT_WORKSPACE_DIR"))
	if workspaceDir == "" {
		workspaceDir = defaultLibvirtWorkspaceDir
	}
	baseImage := strings.TrimSpace(os.Getenv("ARCA_LIBVIRT_BASE_IMAGE"))
	if baseImage == "" {
		baseImage = defaultLibvirtBaseImage
	}
	diskSize := strings.TrimSpace(os.Getenv("ARCA_LIBVIRT_DISK_SIZE"))
	if diskSize == "" {
		diskSize = defaultLibvirtDiskSize
	}
	return &LibvirtRuntime{
		workspaceDir: workspaceDir,
		baseImage:    baseImage,
		diskSize:     diskSize,
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
	if err := r.ensureCloudInitSeed(ctx, machine, workspace, opts); err != nil {
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
		if _, err := runCommand(ctx, "virsh", "define", filepath.Join(workspace, "domain.xml")); err != nil {
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

	if _, err := runCommand(ctx, "virsh", "start", domainName); err != nil {
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
		_, _ = runCommand(ctx, "virsh", "shutdown", domainName)
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
			if _, err := runCommand(ctx, "virsh", "destroy", domainName); err != nil {
				return err
			}
		}
	}

	if _, err := runCommand(ctx, "virsh", "undefine", domainName, "--nvram"); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "no domain with matching name") {
			return err
		}
	}
	_ = os.RemoveAll(filepath.Join(r.workspaceDir, machine.ID))
	return nil
}

func (r *LibvirtRuntime) IsRunning(ctx context.Context, machine db.Machine) (bool, string, error) {
	domainName := r.domainName(machine)
	output, err := runCommand(ctx, "virsh", "domstate", domainName)
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

func (r *LibvirtRuntime) ensureCloudInitSeed(ctx context.Context, machine db.Machine, workspace string, opts RuntimeStartOptions) error {
	userDataPath := filepath.Join(workspace, "user-data")
	metaDataPath := filepath.Join(workspace, "meta-data")
	seedPath := filepath.Join(workspace, "seed.iso")

	userData := cloudInitUserData(machine, opts)
	metaData := fmt.Sprintf("instance-id: %s\nlocal-hostname: arca-%s\n", machine.ID, machine.ID[:12])

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
	_, err := runCommand(ctx, "virsh", "dominfo", domainName)
	if err == nil {
		return true, nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "failed to get domain") {
		return false, nil
	}
	return false, err
}

func (r *LibvirtRuntime) domainXML(domainName, workspace string) string {
	diskPath := filepath.Join(workspace, "disk.qcow2")
	seedPath := filepath.Join(workspace, "seed.iso")
	return fmt.Sprintf(`<domain type='qemu'>
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
      <source network='default'/>
      <model type='virtio'/>
    </interface>
    <console type='pty'/>
    <serial type='pty'/>
  </devices>
</domain>
`, domainName, diskPath, seedPath)
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s failed: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func cloudInitUserData(machine db.Machine, opts RuntimeStartOptions) string {
	envFile := fmt.Sprintf(`ARCAD_TUNNEL_TOKEN=%s
ARCAD_CONTROL_PLANE_URL=%s
ARCAD_MACHINE_ID=%s
ARCAD_MACHINE_TOKEN=%s
TTYD_PORT=21032
TTYD_BASE_PATH=/__arca/ttyd
BASE_PATH=/__arca/claudecodeui
PORT=21031
VITE_IS_PLATFORM=true
`, shellEscape(opts.TunnelToken), shellEscape(opts.ControlPlaneURL), shellEscape(opts.MachineID), shellEscape(opts.MachineToken))

	entrypointScript := `#!/usr/bin/env bash
set -euo pipefail
mkdir -p /home/arca/www
cat > /home/arca/www/index.html <<'HTML'
<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <title>Arca machine</title>
  </head>
  <body>
    <h1>Arca machine is running</h1>
  </body>
</html>
HTML
exec python3 -m http.server 8080 --directory /home/arca/www --bind 127.0.0.1
`

	installScript := `#!/usr/bin/env bash
set -euxo pipefail
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends bash ca-certificates curl git jq python3 ttyd unzip npm golang-go make g++
id -u arca >/dev/null 2>&1 || useradd --create-home --home-dir /home/arca --shell /bin/bash arca
mkdir -p /workspace /etc/arca /opt/arca
chown arca:arca /workspace
chmod 700 /workspace
if [ ! -x /usr/local/bin/cloudflared ]; then
  arch="$(dpkg --print-architecture)"
  curl -fsSL "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-${arch}" -o /usr/local/bin/cloudflared
  chmod +x /usr/local/bin/cloudflared
fi
if [ ! -x /usr/local/bin/arcad ]; then
  GOBIN=/usr/local/bin go install github.com/ryotarai/arca/cmd/arcad@latest
fi
if [ ! -d /home/arca/claudecodeui ]; then
  curl -fsSL https://github.com/ryotarai/claudecodeui/archive/refs/tags/ryotarai-v1.21.0-2.zip -o /tmp/claudecodeui.zip
  unzip -q /tmp/claudecodeui.zip -d /tmp
  mv /tmp/claudecodeui-* /home/arca/claudecodeui
  rm -f /tmp/claudecodeui.zip
fi
cd /home/arca/claudecodeui
npm ci
npm run build
chown -R arca:arca /home/arca
chmod +x /usr/local/bin/arca-entrypoint.sh
systemctl daemon-reload
systemctl enable --now arca-http.service arca-arcad.service arca-ttyd.service arca-claudecodeui.service
`

	return fmt.Sprintf(`#cloud-config
package_update: false
write_files:
  - path: /etc/arca/arcad.env
    permissions: "0600"
    owner: root:root
    encoding: b64
    content: %s
  - path: /usr/local/bin/arca-entrypoint.sh
    permissions: "0755"
    owner: root:root
    encoding: b64
    content: %s
  - path: /usr/local/bin/arca-machine-install.sh
    permissions: "0755"
    owner: root:root
    encoding: b64
    content: %s
  - path: /etc/systemd/system/arca-http.service
    permissions: "0644"
    owner: root:root
    content: |
      [Unit]
      Description=Arca machine sample HTTP service
      After=network-online.target
      Wants=network-online.target
      [Service]
      Type=simple
      ExecStart=/usr/local/bin/arca-entrypoint.sh
      Restart=always
      User=arca
      Group=arca
      [Install]
      WantedBy=multi-user.target
  - path: /etc/systemd/system/arca-arcad.service
    permissions: "0644"
    owner: root:root
    content: |
      [Unit]
      Description=Arca daemon
      After=network-online.target
      Wants=network-online.target
      [Service]
      Type=simple
      EnvironmentFile=/etc/arca/arcad.env
      ExecStart=/usr/local/bin/arcad
      Restart=always
      User=root
      Group=root
      [Install]
      WantedBy=multi-user.target
  - path: /etc/systemd/system/arca-ttyd.service
    permissions: "0644"
    owner: root:root
    content: |
      [Unit]
      Description=Arca ttyd
      After=network-online.target
      Wants=network-online.target
      [Service]
      Type=simple
      EnvironmentFile=/etc/arca/arcad.env
      ExecStart=/usr/bin/ttyd -p ${TTYD_PORT} -b ${TTYD_BASE_PATH} bash
      Restart=always
      User=arca
      Group=arca
      [Install]
      WantedBy=multi-user.target
  - path: /etc/systemd/system/arca-claudecodeui.service
    permissions: "0644"
    owner: root:root
    content: |
      [Unit]
      Description=Arca ClaudeCode UI
      After=network-online.target
      Wants=network-online.target
      [Service]
      Type=simple
      EnvironmentFile=/etc/arca/arcad.env
      WorkingDirectory=/home/arca/claudecodeui
      ExecStart=/usr/bin/node /home/arca/claudecodeui/server/index.js
      Restart=always
      User=arca
      Group=arca
      [Install]
      WantedBy=multi-user.target
runcmd:
  - ["/usr/local/bin/arca-machine-install.sh"]
`, base64.StdEncoding.EncodeToString([]byte(envFile)), base64.StdEncoding.EncodeToString([]byte(entrypointScript)), base64.StdEncoding.EncodeToString([]byte(installScript)))
}

func shellEscape(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "\n", "")
}
