package machine

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
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
	defaultLibvirtArcadGOOS    = "linux"
	defaultLibvirtArcadGOARCH  = "amd64"
)

type LibvirtRuntime struct {
	workspaceDir string
	baseImage    string
	diskSize     string
	uri          string
	arcadGOOS    string
	arcadGOARCH  string
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
	uri := strings.TrimSpace(os.Getenv("ARCA_LIBVIRT_URI"))
	if uri == "" {
		uri = defaultLibvirtURI
	}
	arcadGOOS := strings.TrimSpace(os.Getenv("ARCA_LIBVIRT_ARCAD_GOOS"))
	if arcadGOOS == "" {
		arcadGOOS = defaultLibvirtArcadGOOS
	}
	arcadGOARCH := strings.TrimSpace(os.Getenv("ARCA_LIBVIRT_ARCAD_GOARCH"))
	if arcadGOARCH == "" {
		arcadGOARCH = defaultLibvirtArcadGOARCH
	}
	return &LibvirtRuntime{
		workspaceDir: workspaceDir,
		baseImage:    baseImage,
		diskSize:     diskSize,
		uri:          uri,
		arcadGOOS:    arcadGOOS,
		arcadGOARCH:  arcadGOARCH,
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
	arcadBinaryBase64, err := r.buildArcadBinaryBase64(ctx, workspace)
	if err != nil {
		return "", err
	}
	startupNonce := time.Now().UTC().Format("20060102T150405")
	if err := r.ensureCloudInitSeed(ctx, machine, workspace, opts, arcadBinaryBase64, startupNonce); err != nil {
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

func (r *LibvirtRuntime) WaitReady(ctx context.Context, machine db.Machine, instanceID string) error {
	domainName := firstNonEmpty(instanceID, machine.ContainerID, r.domainName(machine))
	ip, err := r.waitDomainIPv4(ctx, domainName)
	if err != nil {
		return err
	}
	return waitHTTPReady(ctx, fmt.Sprintf("http://%s:21030/__arca/readyz", ip))
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

func (r *LibvirtRuntime) ensureCloudInitSeed(ctx context.Context, machine db.Machine, workspace string, opts RuntimeStartOptions, arcadBinaryBase64, startupNonce string) error {
	userDataPath := filepath.Join(workspace, "user-data")
	metaDataPath := filepath.Join(workspace, "meta-data")
	seedPath := filepath.Join(workspace, "seed.iso")

	userData := cloudInitUserData(machine, opts, arcadBinaryBase64)
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

func (r *LibvirtRuntime) buildArcadBinaryBase64(ctx context.Context, workspace string) (string, error) {
	arcadPath := filepath.Join(workspace, "arcad")
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

func (r *LibvirtRuntime) isDomainDefined(ctx context.Context, domainName string) (bool, error) {
	_, err := r.runVirsh(ctx, "dominfo", domainName)
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
      <source network='default'/>
      <model type='virtio'/>
    </interface>
    <console type='pty'/>
    <serial type='pty'/>
  </devices>
</domain>
`, r.domainType(), domainName, diskPath, seedPath)
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

func (r *LibvirtRuntime) waitDomainIPv4(ctx context.Context, domainName string) (string, error) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var lastErr error
	for {
		ip, err := r.domainIPv4(ctx, domainName)
		if err == nil {
			return ip, nil
		}
		lastErr = err

		select {
		case <-ctx.Done():
			if lastErr == nil {
				return "", ctx.Err()
			}
			return "", fmt.Errorf("%w (last error: %v)", ctx.Err(), lastErr)
		case <-ticker.C:
		}
	}
}

func (r *LibvirtRuntime) domainIPv4(ctx context.Context, domainName string) (string, error) {
	output, err := r.runVirsh(ctx, "domifaddr", domainName, "--source", "lease")
	if err != nil {
		return "", err
	}
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 4 {
			continue
		}
		cidr := fields[3]
		addr, _, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		ip4 := addr.To4()
		if ip4 == nil {
			continue
		}
		return ip4.String(), nil
	}
	return "", fmt.Errorf("no ipv4 lease found for domain %s", domainName)
}

func cloudInitUserData(machine db.Machine, opts RuntimeStartOptions, arcadBinaryBase64 string) string {
	envFile := fmt.Sprintf(`ARCAD_TUNNEL_TOKEN=%s
ARCAD_CONTROL_PLANE_URL=%s
ARCAD_MACHINE_ID=%s
ARCAD_MACHINE_TOKEN=%s
ARCAD_STARTUP_SENTINEL=/var/lib/arca/startup.done
ARCAD_TTYD_SOCKET=/run/arca/ttyd.sock
ARCAD_READY_TCP_ENDPOINTS=127.0.0.1:8080
TTYD_SOCKET=/run/arca/ttyd.sock
TTYD_BASE_PATH=/__arca/ttyd
SHELLEY_BINARY_URL=https://github.com/ryotarai/shelley/releases/download/v0.321.967457453-ryotarai/shelley_linux_amd64
SHELLEY_BASE_PATH=/__arca/shelley
SHELLEY_PORT=21032
SHELLEY_DB_PATH=/var/lib/arca/shelley/shelley.db
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
provision_marker="/var/lib/arca/provisioned"

mkdir -p /var/lib/arca
if [ ! -f "$provision_marker" ]; then
  apt-get update
  apt-get install -y --no-install-recommends bash ca-certificates curl git jq python3 tmux ttyd build-essential
  touch "$provision_marker"
fi

id -u arca >/dev/null 2>&1 || useradd --create-home --home-dir /home/arca --shell /bin/bash arca
mkdir -p /workspace /etc/arca /opt/arca
chown arca:arca /workspace
chmod 700 /workspace
if [ ! -x /usr/local/bin/cloudflared ]; then
  arch="$(dpkg --print-architecture)"
  curl -fsSL "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-${arch}" -o /usr/local/bin/cloudflared
  chmod +x /usr/local/bin/cloudflared
fi
chown -R arca:arca /home/arca
chmod +x /usr/local/bin/arca-entrypoint.sh
chmod +x /usr/local/bin/arca-bootstrap.sh
systemctl daemon-reload
systemctl enable --now arca-bootstrap.service
`

	bootstrapScript := `#!/usr/bin/env bash
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive
sentinel="${ARCAD_STARTUP_SENTINEL:-/var/lib/arca/startup.done}"
provision_marker="/var/lib/arca/provisioned"
rm -f "$sentinel"

id -u arca >/dev/null 2>&1 || useradd --create-home --home-dir /home/arca --shell /bin/bash arca
mkdir -p /workspace /etc/arca /opt/arca /var/lib/arca
chown arca:arca /workspace
chmod 700 /workspace

need_packages=0
for cmd in bash curl git jq python3 tmux ttyd; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    need_packages=1
    break
  fi
done
if [ ! -f "$provision_marker" ] || [ "$need_packages" -eq 1 ]; then
  apt-get update
  apt-get install -y --no-install-recommends bash ca-certificates curl git jq python3 tmux ttyd build-essential
  touch "$provision_marker"
fi

if [ ! -x /usr/local/bin/cloudflared ]; then
  arch="$(dpkg --print-architecture)"
  curl -fsSL "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-${arch}" -o /usr/local/bin/cloudflared
  chmod +x /usr/local/bin/cloudflared
fi

if [ ! -x /usr/local/bin/shelley ]; then
  curl -fsSL "${SHELLEY_BINARY_URL}" -o /usr/local/bin/shelley
  chmod +x /usr/local/bin/shelley
fi

mkdir -p /var/lib/arca/shelley
chown -R arca:arca /var/lib/arca/shelley
chown -R arca:arca /home/arca
chmod +x /usr/local/bin/arca-entrypoint.sh /usr/local/bin/arcad

if [ ! -x /home/linuxbrew/.linuxbrew/bin/brew ]; then
  su - arca -c 'CI=1 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"'
fi

brew_shellenv_line='eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"'
touch /home/arca/.bashrc
if ! grep -Fqx "$brew_shellenv_line" /home/arca/.bashrc; then
  printf '\n# Homebrew\n%s\n' "$brew_shellenv_line" >> /home/arca/.bashrc
fi
chown arca:arca /home/arca/.bashrc

if ! su - arca -c 'eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)" && brew list --formula codex >/dev/null 2>&1'; then
  su - arca -c 'eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)" && brew install codex'
fi
if ! su - arca -c 'eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)" && brew list --formula gemini-cli >/dev/null 2>&1'; then
  su - arca -c 'eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)" && brew install gemini-cli'
fi
if ! su - arca -c 'eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)" && brew list --cask claude-code >/dev/null 2>&1'; then
  su - arca -c 'eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)" && brew install --cask claude-code'
fi

systemctl daemon-reload
systemctl enable arca-http.service arca-arcad.service arca-ttyd.service arca-shelley.service
systemctl restart arca-http.service arca-arcad.service arca-ttyd.service arca-shelley.service

for service in arca-http.service arca-arcad.service arca-ttyd.service arca-shelley.service; do
  for _ in $(seq 1 60); do
    if systemctl is-active --quiet "$service"; then
      break
    fi
    sleep 1
  done
  systemctl is-active --quiet "$service"
done

touch "$sentinel"
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
  - path: /usr/local/bin/arca-bootstrap.sh
    permissions: "0755"
    owner: root:root
    encoding: b64
    content: %s
  - path: /usr/local/bin/arcad
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
      RuntimeDirectory=arca
      ExecStartPre=/usr/bin/rm -f ${TTYD_SOCKET}
      ExecStart=/usr/bin/ttyd -W -i ${TTYD_SOCKET} -U arca:arca -b ${TTYD_BASE_PATH} tmux new-session -A -s arca
      Restart=always
      User=arca
      Group=arca
      [Install]
      WantedBy=multi-user.target
  - path: /etc/systemd/system/arca-bootstrap.service
    permissions: "0644"
    owner: root:root
    content: |
      [Unit]
      Description=Arca machine bootstrap
      After=network-online.target
      Wants=network-online.target
      [Service]
      Type=oneshot
      EnvironmentFile=/etc/arca/arcad.env
      ExecStart=/usr/local/bin/arca-bootstrap.sh
      RemainAfterExit=true
      [Install]
      WantedBy=multi-user.target
  - path: /etc/systemd/system/arca-shelley.service
    permissions: "0644"
    owner: root:root
    content: |
      [Unit]
      Description=Arca Shelley
      After=network-online.target
      Wants=network-online.target
      [Service]
      Type=simple
      EnvironmentFile=/etc/arca/arcad.env
      ExecStart=/usr/local/bin/shelley -db ${SHELLEY_DB_PATH} serve -port ${SHELLEY_PORT} -base-path ${SHELLEY_BASE_PATH}
      Restart=always
      User=arca
      Group=arca
      [Install]
      WantedBy=multi-user.target
runcmd:
  - ["/usr/local/bin/arca-machine-install.sh"]
`, base64.StdEncoding.EncodeToString([]byte(envFile)), base64.StdEncoding.EncodeToString([]byte(entrypointScript)), base64.StdEncoding.EncodeToString([]byte(installScript)), base64.StdEncoding.EncodeToString([]byte(bootstrapScript)), arcadBinaryBase64)
}

func shellEscape(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "\n", "")
}
