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
	envFile := fmt.Sprintf(`ARCAD_CONTROL_PLANE_URL=%s
ARCAD_AUTHORIZE_URL=%s
ARCAD_MACHINE_ID=%s
ARCAD_MACHINE_TOKEN=%s
ARCAD_STARTUP_SENTINEL=/var/lib/arca/startup.done
ARCAD_TTYD_SOCKET=/run/arca/ttyd.sock
ARCAD_READY_TCP_ENDPOINTS=127.0.0.1:21032
ARCA_DAEMON_USER=%s
ARCA_INTERACTIVE_USER=%s
ARCA_AGENT_ENDPOINT_URL=%s
TTYD_SOCKET=/run/arca/ttyd.sock
TTYD_BASE_PATH=/__arca/ttyd
SHELLEY_BINARY_URL_BASE=https://github.com/ryotarai/shelley/releases/download/v0.321.967457453-ryotarai/shelley_linux
SHELLEY_BASE_PATH=/__arca/shelley
SHELLEY_PORT=21032
SHELLEY_DB_PATH=/var/lib/arca/shelley/shelley.db
`, shellEscape(opts.ControlPlaneURL), shellEscape(opts.AuthorizeURL), shellEscape(opts.MachineID), shellEscape(opts.MachineToken), daemonUser, interactiveUser, shellEscape(agentEndpoint))

	// Minimal install script: download arcad binary and start the root service.
	// All other provisioning (packages, users, tools) is handled by arcad's
	// idempotent setup phase.
	installScript := `#!/usr/bin/env bash
set -euxo pipefail
mkdir -p /var/lib/arca /etc/arca

arch="$(dpkg --print-architecture)"
case "$arch" in
  amd64) goarch="amd64" ;;
  arm64) goarch="arm64" ;;
  *) echo "unsupported architecture: $arch"; exit 1 ;;
esac

source /etc/arca/arcad.env

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

systemctl daemon-reload
systemctl enable --now arca-arcad.service
`

	return fmt.Sprintf(`#cloud-config
package_update: false
write_files:
  - path: /etc/arca/arcad.env
    permissions: "0640"
    owner: root:root
    encoding: b64
    content: %s
  - path: /usr/local/bin/arca-install.sh
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
      Description=Arca daemon (root)
      After=network-online.target
      Wants=network-online.target
      [Service]
      Type=simple
      EnvironmentFile=/etc/arca/arcad.env
      ExecStart=/usr/local/bin/arcad
      Restart=always
      [Install]
      WantedBy=multi-user.target
runcmd:
  - ["/usr/local/bin/arca-install.sh"]
`, base64.StdEncoding.EncodeToString([]byte(envFile)), base64.StdEncoding.EncodeToString([]byte(installScript)), base64.StdEncoding.EncodeToString([]byte(startupScript)))
}

func shellEscape(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "\n", "")
}
