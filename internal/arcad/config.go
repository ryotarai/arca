package arcad

import (
	"fmt"
	"os"
	"time"
)

// Config holds arcad runtime configuration.
type Config struct {
	ControlPlaneURL     string
	AuthorizeURL        string
	MachineID           string
	MachineToken        string
	ListenAddr          string
	UpstreamURL         string
	TTydSocket          string
	SessionCookie       string
	StartupSentinel     string
	ReadyEndpoints      string
	ReadyReportInterval time.Duration
	ShelleyPort         string
}

func ConfigFromEnv() (Config, error) {
	cfg := Config{
		ControlPlaneURL: os.Getenv("ARCAD_CONTROL_PLANE_URL"),
		AuthorizeURL:    os.Getenv("ARCAD_AUTHORIZE_URL"),
		MachineID:       os.Getenv("ARCAD_MACHINE_ID"),
		MachineToken:    os.Getenv("ARCAD_MACHINE_TOKEN"),
		ListenAddr:      os.Getenv("ARCAD_LISTEN_ADDR"),
		UpstreamURL:     os.Getenv("ARCAD_UPSTREAM_URL"),
		TTydSocket:      os.Getenv("ARCAD_TTYD_SOCKET"),
		SessionCookie:   os.Getenv("ARCAD_SESSION_COOKIE_NAME"),
		StartupSentinel: os.Getenv("ARCAD_STARTUP_SENTINEL"),
		ReadyEndpoints:  os.Getenv("ARCAD_READY_TCP_ENDPOINTS"),
	}
	readyReportInterval := os.Getenv("ARCAD_READY_REPORT_INTERVAL")
	if readyReportInterval == "" {
		cfg.ReadyReportInterval = 10 * time.Second
	} else {
		parsed, err := time.ParseDuration(readyReportInterval)
		if err != nil || parsed <= 0 {
			return Config{}, fmt.Errorf("ARCAD_READY_REPORT_INTERVAL must be a positive duration")
		}
		cfg.ReadyReportInterval = parsed
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":21030"
	}
	if cfg.UpstreamURL == "" {
		cfg.UpstreamURL = "http://127.0.0.1:11030"
	}
	if cfg.TTydSocket == "" {
		cfg.TTydSocket = "/run/arca/ttyd.sock"
	}
	if cfg.SessionCookie == "" {
		cfg.SessionCookie = "arcad_session"
	}
	if cfg.StartupSentinel == "" {
		cfg.StartupSentinel = "/var/lib/arca/startup.done"
	}
	cfg.ShelleyPort = os.Getenv("SHELLEY_PORT")
	if cfg.ShelleyPort == "" {
		cfg.ShelleyPort = "21032"
	}
	if cfg.ControlPlaneURL == "" {
		return Config{}, fmt.Errorf("ARCAD_CONTROL_PLANE_URL is required")
	}
	if cfg.MachineID == "" {
		return Config{}, fmt.Errorf("ARCAD_MACHINE_ID is required")
	}
	return cfg, nil
}
