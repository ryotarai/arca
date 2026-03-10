package machine

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/ryotarai/arca/internal/db"
)

func cloudInitUserData(machine db.Machine, opts RuntimeStartOptions) string {
	const (
		daemonUser      = "arcad"
		interactiveUser = "arcauser"
		agentEndpoint   = "http://localhost:11030"
	)
	startupScript := opts.StartupScript
	if strings.TrimSpace(startupScript) == "" {
		startupScript = "exit 0\n"
	}
	startupScript = "#!/usr/bin/env bash\nset -euo pipefail\n" + startupScript
	authorizedKeys := strings.TrimSpace(strings.Join(opts.InteractiveSSHPubKeys, "\n"))
	if authorizedKeys != "" {
		authorizedKeys += "\n"
	}
	authorizedKeysBase64 := base64.StdEncoding.EncodeToString([]byte(authorizedKeys))
	agentGuidelineSectionBase64 := base64.StdEncoding.EncodeToString([]byte(agentGuidelineSection(agentEndpoint)))

	envFile := fmt.Sprintf(`ARCAD_TUNNEL_TOKEN=%s
ARCAD_CONTROL_PLANE_URL=%s
ARCAD_AUTHORIZE_URL=%s
ARCAD_MACHINE_ID=%s
ARCAD_MACHINE_TOKEN=%s
ARCAD_STARTUP_SENTINEL=/var/lib/arca/startup.done
ARCAD_TTYD_SOCKET=/run/arca/ttyd.sock
ARCAD_READY_TCP_ENDPOINTS=127.0.0.1:21032
ARCA_DAEMON_USER=%s
ARCA_INTERACTIVE_USER=%s
ARCA_INTERACTIVE_AUTHORIZED_KEYS_B64=%s
ARCA_AGENT_ENDPOINT_URL=%s
TTYD_SOCKET=/run/arca/ttyd.sock
TTYD_BASE_PATH=/__arca/ttyd
SHELLEY_BINARY_URL_BASE=https://github.com/ryotarai/shelley/releases/download/v0.321.967457453-ryotarai/shelley_linux
SHELLEY_BASE_PATH=/__arca/shelley
SHELLEY_PORT=21032
SHELLEY_DB_PATH=/var/lib/arca/shelley/shelley.db
`, shellEscape(opts.TunnelToken), shellEscape(opts.ControlPlaneURL), shellEscape(opts.AuthorizeURL), shellEscape(opts.MachineID), shellEscape(opts.MachineToken), daemonUser, interactiveUser, shellEscape(authorizedKeysBase64), shellEscape(agentEndpoint))

	installScript := `#!/usr/bin/env bash
set -euxo pipefail
export DEBIAN_FRONTEND=noninteractive
provision_marker="/var/lib/arca/provisioned"
daemon_user="${ARCA_DAEMON_USER:-arcad}"
interactive_user="${ARCA_INTERACTIVE_USER:-arcauser}"
interactive_home="/home/${interactive_user}"

mkdir -p /var/lib/arca /etc/arca /opt/arca /workspace
if [ ! -f "$provision_marker" ]; then
  apt-get update
  apt-get install -y --no-install-recommends bash ca-certificates curl git jq python3 tmux ttyd build-essential sudo
  touch "$provision_marker"
fi

getent group arca >/dev/null 2>&1 || groupadd --system arca
id -u "$daemon_user" >/dev/null 2>&1 || useradd --system --gid arca --home-dir /nonexistent --shell /usr/sbin/nologin "$daemon_user"
id -u "$interactive_user" >/dev/null 2>&1 || useradd --create-home --home-dir "$interactive_home" --shell /bin/bash --gid arca "$interactive_user"
cat > /etc/sudoers.d/90-arcauser <<EOF
${interactive_user} ALL=(ALL) NOPASSWD:ALL
EOF
chmod 0440 /etc/sudoers.d/90-arcauser
chown "$interactive_user":arca /workspace
chmod 700 /workspace
if [ ! -x /usr/local/bin/cloudflared ]; then
  arch="$(dpkg --print-architecture)"
  curl -fsSL "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-${arch}" -o /usr/local/bin/cloudflared
  chmod +x /usr/local/bin/cloudflared
fi
chown root:arca /etc/arca/arcad.env
chmod 0640 /etc/arca/arcad.env
chown -R "$interactive_user":arca "$interactive_home"
chmod +x /usr/local/bin/arca-bootstrap.sh
systemctl daemon-reload
systemctl enable --now arca-bootstrap.service
`

	bootstrapScript := `#!/usr/bin/env bash
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive
sentinel="${ARCAD_STARTUP_SENTINEL:-/var/lib/arca/startup.done}"
provision_marker="/var/lib/arca/provisioned"
daemon_user="${ARCA_DAEMON_USER:-arcad}"
interactive_user="${ARCA_INTERACTIVE_USER:-arcauser}"
interactive_home="/home/${interactive_user}"
interactive_ssh_dir="${interactive_home}/.ssh"
authorized_keys_path="${interactive_ssh_dir}/authorized_keys"
rm -f "$sentinel"

mkdir -p /workspace /etc/arca /opt/arca /var/lib/arca
getent group arca >/dev/null 2>&1 || groupadd --system arca
id -u "$daemon_user" >/dev/null 2>&1 || useradd --system --gid arca --home-dir /nonexistent --shell /usr/sbin/nologin "$daemon_user"
id -u "$interactive_user" >/dev/null 2>&1 || useradd --create-home --home-dir "$interactive_home" --shell /bin/bash --gid arca "$interactive_user"
chown "$interactive_user":arca /workspace
chmod 700 /workspace

need_packages=0
for cmd in bash curl git jq python3 tmux ttyd sudo; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    need_packages=1
    break
  fi
done
if [ ! -f "$provision_marker" ] || [ "$need_packages" -eq 1 ]; then
  apt-get update
  apt-get install -y --no-install-recommends bash ca-certificates curl git jq python3 tmux ttyd build-essential sudo
  touch "$provision_marker"
fi
cat > /etc/sudoers.d/90-arcauser <<EOF
${interactive_user} ALL=(ALL) NOPASSWD:ALL
EOF
chmod 0440 /etc/sudoers.d/90-arcauser

if [ ! -x /usr/local/bin/cloudflared ]; then
  arch="$(dpkg --print-architecture)"
  curl -fsSL "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-${arch}" -o /usr/local/bin/cloudflared
  chmod +x /usr/local/bin/cloudflared
fi

if [ ! -x /usr/local/bin/shelley ]; then
  shelley_arch="$(dpkg --print-architecture)"
  curl -fsSL "${SHELLEY_BINARY_URL_BASE}_${shelley_arch}" -o /usr/local/bin/shelley
  chmod +x /usr/local/bin/shelley
fi

mkdir -p /var/lib/arca/shelley
chown -R "$interactive_user":arca /var/lib/arca/shelley
chown root:arca /etc/arca/arcad.env
chmod 0640 /etc/arca/arcad.env
mkdir -p "$interactive_ssh_dir"
chown -R "$interactive_user":arca "$interactive_home"
chmod 700 "$interactive_ssh_dir"
keys_tmp="$(mktemp)"
if [ -n "${ARCA_INTERACTIVE_AUTHORIZED_KEYS_B64:-}" ]; then
  printf '%s' "${ARCA_INTERACTIVE_AUTHORIZED_KEYS_B64}" | base64 -d > "$keys_tmp"
else
  : > "$keys_tmp"
fi
install -o "$interactive_user" -g arca -m 0600 "$keys_tmp" "$authorized_keys_path"
rm -f "$keys_tmp"
arch="$(dpkg --print-architecture)"
case "$arch" in
  amd64) goarch="amd64" ;;
  arm64) goarch="arm64" ;;
  *) echo "unsupported architecture: $arch"; exit 1 ;;
esac
for attempt in $(seq 1 10); do
  if curl -fsSL \
    -H "Authorization: Bearer ${ARCAD_MACHINE_TOKEN}" \
    "${ARCAD_CONTROL_PLANE_URL}/arcad/download?os=linux&arch=${goarch}" \
    -o /usr/local/bin/arcad; then
    break
  fi
  echo "arcad download attempt $attempt failed, retrying in 5s..."
  sleep 5
done
chmod +x /usr/local/bin/arcad

/usr/bin/env bash /usr/local/bin/arca-user-startup.sh

write_agent_guideline_file() {
  local target_path="$1"
  local managed_section_b64="$2"
  python3 - "$target_path" "$managed_section_b64" <<'PY'
import base64
import pathlib
import sys

target_path = pathlib.Path(sys.argv[1])
managed_section = base64.b64decode(sys.argv[2]).decode("utf-8")
start_marker = "` + agentGuidelineMarkerStart + `"
end_marker = "` + agentGuidelineMarkerEnd + `"

if target_path.exists():
    current = target_path.read_text(encoding="utf-8")
else:
    current = ""

start = current.find(start_marker)
if start >= 0:
    end = current.find(end_marker, start + len(start_marker))
else:
    end = -1

if start >= 0 and end >= 0:
    end += len(end_marker)
    updated = current[:start] + managed_section + current[end:]
else:
    if current and not current.endswith("\n"):
        current += "\n"
    if current and not current.endswith("\n\n"):
        current += "\n"
    updated = current + managed_section

target_path.parent.mkdir(parents=True, exist_ok=True)
target_path.write_text(updated, encoding="utf-8")
PY
}

guideline_section_b64="` + agentGuidelineSectionBase64 + `"
write_agent_guideline_file "${interactive_home}/.claude/CLAUDE.md" "${guideline_section_b64}"
write_agent_guideline_file "${interactive_home}/.codex/AGENTS.md" "${guideline_section_b64}"
write_agent_guideline_file "${interactive_home}/.gemini/GEMINI.md" "${guideline_section_b64}"
chown -R "$interactive_user":arca "${interactive_home}/.claude" "${interactive_home}/.codex" "${interactive_home}/.gemini"


systemctl daemon-reload
systemctl enable arca-arcad.service arca-ttyd.service arca-shelley.service
systemctl restart arca-arcad.service arca-ttyd.service arca-shelley.service

for service in arca-arcad.service arca-ttyd.service arca-shelley.service; do
  for _ in $(seq 1 60); do
    if systemctl is-active --quiet "$service"; then
      break
    fi
    sleep 1
  done
  systemctl is-active --quiet "$service"
done

touch "$sentinel"
# Optional developer tooling should never block readiness.
set +e
if [ ! -x /home/linuxbrew/.linuxbrew/bin/brew ]; then
  su - "$interactive_user" -c 'CI=1 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"'
fi

if [ -x /home/linuxbrew/.linuxbrew/bin/brew ]; then
  brew_shellenv_line='eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"'
  touch "${interactive_home}/.bashrc"
  if ! grep -Fqx "$brew_shellenv_line" "${interactive_home}/.bashrc"; then
    printf '\n# Homebrew\n%s\n' "$brew_shellenv_line" >> "${interactive_home}/.bashrc"
  fi
  chown "$interactive_user":arca "${interactive_home}/.bashrc"

  if ! su - "$interactive_user" -c 'eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)" && brew list --formula codex >/dev/null 2>&1'; then
    su - "$interactive_user" -c 'eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)" && brew install codex'
  fi
  if ! su - "$interactive_user" -c 'eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)" && brew list --formula gemini-cli >/dev/null 2>&1'; then
    su - "$interactive_user" -c 'eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)" && brew install gemini-cli'
  fi
  if ! su - "$interactive_user" -c 'eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)" && brew list --cask claude-code >/dev/null 2>&1'; then
    su - "$interactive_user" -c 'eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)" && brew install --cask claude-code'
  fi
fi
set -e

`

	return fmt.Sprintf(`#cloud-config
package_update: false
write_files:
  - path: /etc/arca/arcad.env
    permissions: "0640"
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
  - path: /usr/local/bin/arca-user-startup.sh
    permissions: "0600"
    owner: root:root
    encoding: b64
    content: %s
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
      User=arcad
      Group=arca
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
      RuntimeDirectoryMode=0770
      UMask=0007
      WorkingDirectory=/home/arcauser
      ExecStartPre=/usr/bin/rm -f ${TTYD_SOCKET}
      ExecStart=/usr/bin/ttyd -W -i ${TTYD_SOCKET} -U arcauser:arca -b ${TTYD_BASE_PATH} tmux new-session -A -s arca
      Restart=always
      User=arcauser
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
      WorkingDirectory=/home/arcauser
      ExecStart=/usr/local/bin/shelley -db ${SHELLEY_DB_PATH} serve -port ${SHELLEY_PORT} -base-path ${SHELLEY_BASE_PATH}
      Restart=always
      User=arcauser
      Group=arca
      [Install]
      WantedBy=multi-user.target
runcmd:
  - ["/usr/local/bin/arca-machine-install.sh"]
`, base64.StdEncoding.EncodeToString([]byte(envFile)), base64.StdEncoding.EncodeToString([]byte(installScript)), base64.StdEncoding.EncodeToString([]byte(bootstrapScript)), base64.StdEncoding.EncodeToString([]byte(startupScript)))
}

func shellEscape(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "\n", "")
}
