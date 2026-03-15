package arcad

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"
)

// SetupConfig holds configuration for the idempotent setup phase.
type SetupConfig struct {
	DaemonUser           string
	InteractiveUser      string
	AuthorizedKeysB64    string
	AgentEndpointURL     string
	ShelleyBinaryURLBase string
	ShelleyBasePath      string
	ShelleyPort          string
	ShelleyDBPath        string
	TTydSocket           string
	TTydBasePath         string
	StartupSentinel      string
	UserStartupScript    string
}

// SetupConfigFromEnv reads setup configuration from environment variables.
func SetupConfigFromEnv() SetupConfig {
	return SetupConfig{
		DaemonUser:           envOrDefault("ARCA_DAEMON_USER", "arcad"),
		InteractiveUser:      envOrDefault("ARCA_INTERACTIVE_USER", "arcauser"),
		AuthorizedKeysB64:    os.Getenv("ARCA_INTERACTIVE_AUTHORIZED_KEYS_B64"),
		AgentEndpointURL:     envOrDefault("ARCA_AGENT_ENDPOINT_URL", "http://localhost:11030"),
		ShelleyBinaryURLBase: os.Getenv("SHELLEY_BINARY_URL_BASE"),
		ShelleyBasePath:      envOrDefault("SHELLEY_BASE_PATH", "/__arca/shelley"),
		ShelleyPort:          envOrDefault("SHELLEY_PORT", "21032"),
		ShelleyDBPath:        envOrDefault("SHELLEY_DB_PATH", "/var/lib/arca/shelley/shelley.db"),
		TTydSocket:           envOrDefault("ARCAD_TTYD_SOCKET", "/run/arca/ttyd.sock"),
		TTydBasePath:         envOrDefault("TTYD_BASE_PATH", "/__arca/ttyd"),
		StartupSentinel:      envOrDefault("ARCAD_STARTUP_SENTINEL", "/var/lib/arca/startup.done"),
		UserStartupScript:    "/usr/local/bin/arca-user-startup.sh",
	}
}

func envOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// RunSetup runs the idempotent provisioning phase.
// Each step checks current state and skips if already satisfied.
func RunSetup(ctx context.Context, setupCfg SetupConfig) error {
	steps := []struct {
		name string
		fn   func(context.Context, SetupConfig) error
	}{
		{"create directories", stepCreateDirectories},
		{"create users and groups", stepCreateUsersAndGroups},
		{"install system packages", stepInstallPackages},
		{"configure sudoers", stepConfigureSudoers},
		{"configure workspace", stepConfigureWorkspace},
		{"download cloudflared", stepDownloadCloudflared},
		{"download shelley", stepDownloadShelley},
		{"create shelley data dir", stepCreateShelleyDataDir},
		{"set env file permissions", stepSetEnvFilePermissions},
		{"deploy SSH keys", stepDeploySSHKeys},
		{"run user startup script", stepRunUserStartupScript},
		{"write agent guidelines", stepWriteAgentGuidelines},
		{"write systemd unit files", stepWriteSystemdUnits},
		{"start dependent services", stepStartServices},
		{"touch startup sentinel", stepTouchSentinel},
		{"install dev tools", stepInstallDevTools},
	}

	for _, step := range steps {
		log.Printf("setup: %s", step.name)
		if err := step.fn(ctx, setupCfg); err != nil {
			return fmt.Errorf("setup step %q failed: %w", step.name, err)
		}
	}

	log.Printf("setup: complete")
	return nil
}

func stepCreateDirectories(_ context.Context, _ SetupConfig) error {
	for _, dir := range []string{"/workspace", "/etc/arca", "/opt/arca", "/var/lib/arca"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	return nil
}

func stepCreateUsersAndGroups(_ context.Context, cfg SetupConfig) error {
	if _, err := user.LookupGroup("arca"); err != nil {
		if err := runCmd("groupadd", "--system", "arca"); err != nil {
			return fmt.Errorf("create arca group: %w", err)
		}
	}

	if _, err := user.Lookup(cfg.DaemonUser); err != nil {
		if err := runCmd("useradd", "--system", "--gid", "arca", "--home-dir", "/nonexistent", "--shell", "/usr/sbin/nologin", cfg.DaemonUser); err != nil {
			return fmt.Errorf("create daemon user: %w", err)
		}
	}

	interactiveHome := "/home/" + cfg.InteractiveUser
	if _, err := user.Lookup(cfg.InteractiveUser); err != nil {
		if err := runCmd("useradd", "--create-home", "--home-dir", interactiveHome, "--shell", "/bin/bash", "--gid", "arca", cfg.InteractiveUser); err != nil {
			return fmt.Errorf("create interactive user: %w", err)
		}
	}

	return runCmd("chown", "-R", cfg.InteractiveUser+":arca", interactiveHome)
}

func stepInstallPackages(_ context.Context, _ SetupConfig) error {
	provisionMarker := "/var/lib/arca/provisioned"

	needPackages := false
	for _, cmd := range []string{"bash", "curl", "git", "jq", "python3", "tmux", "ttyd", "sudo"} {
		if _, err := exec.LookPath(cmd); err != nil {
			needPackages = true
			break
		}
	}
	if _, err := os.Stat(provisionMarker); err != nil {
		needPackages = true
	}

	if !needPackages {
		return nil
	}

	env := []string{"DEBIAN_FRONTEND=noninteractive"}
	if err := runCmdWithEnv(env, "apt-get", "update"); err != nil {
		return fmt.Errorf("apt-get update: %w", err)
	}
	if err := runCmdWithEnv(env, "apt-get", "install", "-y", "--no-install-recommends",
		"bash", "ca-certificates", "curl", "git", "jq", "python3", "tmux", "ttyd", "build-essential", "sudo"); err != nil {
		return fmt.Errorf("apt-get install: %w", err)
	}

	return os.WriteFile(provisionMarker, []byte("1"), 0644)
}

func stepConfigureSudoers(_ context.Context, cfg SetupConfig) error {
	content := cfg.InteractiveUser + " ALL=(ALL) NOPASSWD:ALL\n"
	return os.WriteFile("/etc/sudoers.d/90-arcauser", []byte(content), 0440)
}

func stepConfigureWorkspace(_ context.Context, cfg SetupConfig) error {
	if err := runCmd("chown", cfg.InteractiveUser+":arca", "/workspace"); err != nil {
		return err
	}
	return os.Chmod("/workspace", 0700)
}

func stepDownloadCloudflared(_ context.Context, _ SetupConfig) error {
	if _, err := os.Stat("/usr/local/bin/cloudflared"); err == nil {
		return nil
	}
	arch := detectDpkgArch()
	url := fmt.Sprintf("https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-%s", arch)
	return downloadToPath(url, "/usr/local/bin/cloudflared", 0755)
}

func stepDownloadShelley(_ context.Context, cfg SetupConfig) error {
	if _, err := os.Stat("/usr/local/bin/shelley"); err == nil {
		return nil
	}
	if cfg.ShelleyBinaryURLBase == "" {
		log.Printf("setup: SHELLEY_BINARY_URL_BASE not set, skipping shelley download")
		return nil
	}
	arch := detectDpkgArch()
	url := cfg.ShelleyBinaryURLBase + "_" + arch
	return downloadToPath(url, "/usr/local/bin/shelley", 0755)
}

func stepCreateShelleyDataDir(_ context.Context, cfg SetupConfig) error {
	dir := filepath.Dir(cfg.ShelleyDBPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return runCmd("chown", "-R", cfg.InteractiveUser+":arca", dir)
}

func stepSetEnvFilePermissions(_ context.Context, _ SetupConfig) error {
	if err := runCmd("chown", "root:arca", "/etc/arca/arcad.env"); err != nil {
		return err
	}
	return os.Chmod("/etc/arca/arcad.env", 0640)
}

func stepDeploySSHKeys(_ context.Context, cfg SetupConfig) error {
	interactiveHome := "/home/" + cfg.InteractiveUser
	sshDir := filepath.Join(interactiveHome, ".ssh")

	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return err
	}

	var keys []byte
	if cfg.AuthorizedKeysB64 != "" {
		var err error
		keys, err = base64.StdEncoding.DecodeString(cfg.AuthorizedKeysB64)
		if err != nil {
			return fmt.Errorf("decode authorized keys: %w", err)
		}
	}

	keysPath := filepath.Join(sshDir, "authorized_keys")
	if err := os.WriteFile(keysPath, keys, 0600); err != nil {
		return err
	}

	return runCmd("chown", "-R", cfg.InteractiveUser+":arca", sshDir)
}

func stepRunUserStartupScript(_ context.Context, cfg SetupConfig) error {
	if _, err := os.Stat(cfg.UserStartupScript); err != nil {
		return nil
	}
	cmd := exec.Command("/usr/bin/env", "bash", cfg.UserStartupScript)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func stepWriteAgentGuidelines(_ context.Context, cfg SetupConfig) error {
	interactiveHome := "/home/" + cfg.InteractiveUser
	section := buildAgentGuidelineSection(cfg.AgentEndpointURL)

	targets := []string{
		filepath.Join(interactiveHome, ".claude", "CLAUDE.md"),
		filepath.Join(interactiveHome, ".codex", "AGENTS.md"),
		filepath.Join(interactiveHome, ".gemini", "GEMINI.md"),
		filepath.Join(interactiveHome, ".config", "AGENTS.md"),
	}

	for _, target := range targets {
		if err := writeGuidelineFile(target, section); err != nil {
			log.Printf("setup: failed to write agent guideline to %s: %v", target, err)
		}
	}

	for _, dir := range []string{".claude", ".codex", ".gemini", ".config"} {
		dirPath := filepath.Join(interactiveHome, dir)
		if _, err := os.Stat(dirPath); err == nil {
			_ = runCmd("chown", "-R", cfg.InteractiveUser+":arca", dirPath)
		}
	}

	return nil
}

const (
	guidelineMarkerStart = "<!-- ARCA:AGENT_GUIDELINE_START -->"
	guidelineMarkerEnd   = "<!-- ARCA:AGENT_GUIDELINE_END -->"
)

func buildAgentGuidelineSection(endpointURL string) string {
	var b strings.Builder
	b.WriteString(guidelineMarkerStart)
	b.WriteString("\n")
	b.WriteString("# Arca Agent Guidelines\n\n")
	b.WriteString("This section is managed by Arca and is safe to re-generate.\n\n")
	b.WriteString("- Run your application HTTP server on `:11030`.\n")
	b.WriteString("- Endpoint URL inside this machine: `" + strings.TrimSpace(endpointURL) + "`.\n")
	b.WriteString("- Requests to the endpoint URL are delivered to port `11030` on this machine.\n")
	b.WriteString("- The server process is started and supervised by `systemd`.\n")
	b.WriteString("- Visibility scope (`owner only`, `specific users`, `all arca users`, `internet public`) is configured in the arca app (server).\n")
	b.WriteString("\n")
	b.WriteString("You can add your own notes outside this managed block.\n")
	b.WriteString(guidelineMarkerEnd)
	b.WriteString("\n")
	return b.String()
}

func writeGuidelineFile(targetPath, managedSection string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return err
	}
	existing := ""
	if data, err := os.ReadFile(targetPath); err == nil {
		existing = string(data)
	}
	return os.WriteFile(targetPath, []byte(replaceOrAppendGuideline(existing, managedSection)), 0644)
}

func replaceOrAppendGuideline(existing, managedSection string) string {
	start := strings.Index(existing, guidelineMarkerStart)
	if start >= 0 {
		searchFrom := start + len(guidelineMarkerStart)
		if endRel := strings.Index(existing[searchFrom:], guidelineMarkerEnd); endRel >= 0 {
			end := searchFrom + endRel + len(guidelineMarkerEnd)
			return existing[:start] + managedSection + existing[end:]
		}
	}

	if strings.TrimSpace(existing) == "" {
		return managedSection
	}
	if !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	if !strings.HasSuffix(existing, "\n\n") {
		existing += "\n"
	}
	return existing + managedSection
}

func stepWriteSystemdUnits(_ context.Context, cfg SetupConfig) error {
	arcadUserUnit := fmt.Sprintf(`[Unit]
Description=Arca daemon (user)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=/etc/arca/arcad.env
ExecStart=/usr/local/bin/arcad --user
Restart=always
User=%s
Group=arca

[Install]
WantedBy=multi-user.target
`, cfg.DaemonUser)

	ttydUnit := fmt.Sprintf(`[Unit]
Description=Arca ttyd
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=/etc/arca/arcad.env
RuntimeDirectory=arca
RuntimeDirectoryMode=0770
UMask=0007
WorkingDirectory=/home/%s
ExecStartPre=/usr/bin/rm -f %s
ExecStart=/usr/bin/ttyd -W -i %s -U %s:arca -b %s tmux new-session -A -s arca
Restart=always
User=%s
Group=arca

[Install]
WantedBy=multi-user.target
`, cfg.InteractiveUser, cfg.TTydSocket, cfg.TTydSocket, cfg.InteractiveUser, cfg.TTydBasePath, cfg.InteractiveUser)

	shelleyUnit := fmt.Sprintf(`[Unit]
Description=Arca Shelley
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=/etc/arca/arcad.env
WorkingDirectory=/home/%s
ExecStart=/usr/local/bin/shelley -db %s serve -port %s -base-path %s
Restart=always
User=%s
Group=arca

[Install]
WantedBy=multi-user.target
`, cfg.InteractiveUser, cfg.ShelleyDBPath, cfg.ShelleyPort, cfg.ShelleyBasePath, cfg.InteractiveUser)

	units := map[string]string{
		"/etc/systemd/system/arca-arcad-user.service": arcadUserUnit,
		"/etc/systemd/system/arca-ttyd.service":       ttydUnit,
		"/etc/systemd/system/arca-shelley.service":    shelleyUnit,
	}

	for path, content := range units {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}

	return runCmd("systemctl", "daemon-reload")
}

func stepStartServices(_ context.Context, _ SetupConfig) error {
	services := []string{"arca-arcad-user.service", "arca-ttyd.service", "arca-shelley.service"}

	enableArgs := append([]string{"enable"}, services...)
	if err := runCmd("systemctl", enableArgs...); err != nil {
		return fmt.Errorf("enable services: %w", err)
	}

	restartArgs := append([]string{"restart"}, services...)
	if err := runCmd("systemctl", restartArgs...); err != nil {
		return fmt.Errorf("restart services: %w", err)
	}

	for _, svc := range services {
		for i := 0; i < 60; i++ {
			if exec.Command("systemctl", "is-active", "--quiet", svc).Run() == nil {
				break
			}
			time.Sleep(1 * time.Second)
		}
		if err := exec.Command("systemctl", "is-active", "--quiet", svc).Run(); err != nil {
			return fmt.Errorf("service %s did not become active", svc)
		}
	}

	return nil
}

func stepTouchSentinel(_ context.Context, cfg SetupConfig) error {
	return os.WriteFile(cfg.StartupSentinel, []byte("1"), 0644)
}

func stepInstallDevTools(_ context.Context, cfg SetupConfig) error {
	interactiveUser := cfg.InteractiveUser

	if _, err := os.Stat("/home/linuxbrew/.linuxbrew/bin/brew"); err != nil {
		log.Printf("setup: installing Homebrew...")
		cmd := exec.Command("su", "-", interactiveUser, "-c",
			`CI=1 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"`)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Printf("setup: Homebrew installation failed (non-fatal): %v", err)
			return nil
		}
	}

	if _, err := os.Stat("/home/linuxbrew/.linuxbrew/bin/brew"); err != nil {
		return nil
	}

	bashrcPath := "/home/" + interactiveUser + "/.bashrc"
	shellenvLine := `eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"`
	if data, err := os.ReadFile(bashrcPath); err != nil || !strings.Contains(string(data), shellenvLine) {
		f, err := os.OpenFile(bashrcPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			fmt.Fprintf(f, "\n# Homebrew\n%s\n", shellenvLine)
			f.Close()
			_ = runCmd("chown", interactiveUser+":arca", bashrcPath)
		}
	}

	brewEnv := `eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"`
	for _, pkg := range []struct{ name, typ string }{
		{"codex", "formula"},
		{"gemini-cli", "formula"},
		{"claude-code", "cask"},
	} {
		listFlag := "--formula"
		installFlag := ""
		if pkg.typ == "cask" {
			listFlag = "--cask"
			installFlag = "--cask"
		}

		checkCmd := fmt.Sprintf(`%s && brew list %s %s >/dev/null 2>&1`, brewEnv, listFlag, pkg.name)
		if exec.Command("su", "-", interactiveUser, "-c", checkCmd).Run() != nil {
			installCmd := fmt.Sprintf(`%s && brew install %s %s`, brewEnv, installFlag, pkg.name)
			cmd := exec.Command("su", "-", interactiveUser, "-c", installCmd)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				log.Printf("setup: brew install %s failed (non-fatal): %v", pkg.name, err)
			}
		}
	}

	return nil
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runCmdWithEnv(env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func detectDpkgArch() string {
	out, err := exec.Command("dpkg", "--print-architecture").Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return "amd64"
}

func downloadToPath(url, destPath string, perm os.FileMode) error {
	tmpFile, err := os.CreateTemp(filepath.Dir(destPath), ".download-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	cmd := exec.Command("curl", "-fsSL", url, "-o", tmpPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}

	if err := os.Chmod(tmpPath, perm); err != nil {
		return err
	}
	return os.Rename(tmpPath, destPath)
}
