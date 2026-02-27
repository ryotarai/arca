# arcad supervisor model assumptions

`arcad` is designed to run under an external supervisor (for example `systemd`, `s6-overlay`, or `runit`) and is not expected to be PID 1.

Assumptions in this MVP:

- Supervisor starts `arcad` and restarts it after crashes.
- Supervisor injects required environment variables:
  - `ARCAD_CONTROL_PLANE_URL`
  - `ARCAD_MACHINE_ID`
  - `ARCAD_MACHINE_TOKEN`
  - `ARCAD_TUNNEL_TOKEN`
  - optional: `ARCAD_LISTEN_ADDR` and `ARCAD_SESSION_COOKIE_NAME`
- `arcad` manages `cloudflared` as a child process with a restart-on-exit loop.
- Other workspace services (`ttyd`, agent process, user app processes) are managed separately by supervisor and exposed through localhost targets registered in control-plane exposure metadata.
- `arcad` trusts the control plane for exposure metadata and ticket verification; local session cookies are scoped per machine host.
- Graceful shutdown is signal-driven (`SIGTERM`/`SIGINT`) and lets the HTTP server drain connections before exit.

Non-goals in this MVP:

- `arcad` does not implement full init/supervisor behavior.
- No zero-downtime binary self-upgrade or file-descriptor handoff yet.
