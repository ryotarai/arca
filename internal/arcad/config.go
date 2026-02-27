package arcad

import (
	"fmt"
	"os"
)

// Config holds arcad runtime configuration.
type Config struct {
	ControlPlaneURL string
	MachineID       string
	MachineToken    string
	TunnelToken     string
	ListenAddr      string
	SessionCookie   string
}

func ConfigFromEnv() (Config, error) {
	cfg := Config{
		ControlPlaneURL: os.Getenv("ARCAD_CONTROL_PLANE_URL"),
		MachineID:       os.Getenv("ARCAD_MACHINE_ID"),
		MachineToken:    os.Getenv("ARCAD_MACHINE_TOKEN"),
		TunnelToken:     os.Getenv("ARCAD_TUNNEL_TOKEN"),
		ListenAddr:      os.Getenv("ARCAD_LISTEN_ADDR"),
		SessionCookie:   os.Getenv("ARCAD_SESSION_COOKIE_NAME"),
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":80"
	}
	if cfg.SessionCookie == "" {
		cfg.SessionCookie = "arcad_session"
	}
	if cfg.ControlPlaneURL == "" {
		return Config{}, fmt.Errorf("ARCAD_CONTROL_PLANE_URL is required")
	}
	if cfg.MachineID == "" {
		return Config{}, fmt.Errorf("ARCAD_MACHINE_ID is required")
	}
	if cfg.MachineToken == "" {
		return Config{}, fmt.Errorf("ARCAD_MACHINE_TOKEN is required")
	}
	if cfg.TunnelToken == "" {
		return Config{}, fmt.Errorf("ARCAD_TUNNEL_TOKEN is required")
	}
	return cfg, nil
}
