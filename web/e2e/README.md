# E2E test guide

## Prerequisites

The critical user journey test requires **LXD** for container-based machine management.

### LXD setup

1. Install and initialize LXD:

```bash
sudo snap install lxd
sudo lxd init --auto
```

2. Add your user to the `lxd` group (log out and back in after):

```bash
sudo usermod -aG lxd $USER
```

3. Verify LXD is working:

```bash
lxc list
```

The E2E test inserts an LXD runtime record into the test database with the default endpoint (`https://localhost:8443`). The `lxc` CLI uses the local unix socket by default, so no TLS configuration is needed.

## Default local run

Run UI E2E tests without real Cloudflare credentials:

```bash
npm --prefix web run e2e
```

Behavior:
- Server starts with `ARCA_SKIP_CLOUDFLARE_VALIDATION=1` when Cloudflare env vars are not set.
- Cloudflare-specific critical journey tests are skipped with guidance.

## Cloudflare-backed critical journey run

Set all required env vars before running Playwright:

```bash
export CLOUDFLARE_TOKEN="..."
export CLOUDFLARE_ACCOUNT_ID="..."
export CLOUDFLARE_ZONE_ID="..."
export BASE_DOMAIN="example.com"
export DOMAIN_PREFIX="arca-"
npm --prefix web run e2e
```

Required env vars:
- `CLOUDFLARE_TOKEN`
- `CLOUDFLARE_ACCOUNT_ID`
- `CLOUDFLARE_ZONE_ID`
- `BASE_DOMAIN`
- `DOMAIN_PREFIX`

When all are present:
- Setup uses these values.
- Cloudflare token validation is executed in the critical journey test.
- The test checks `login -> create machine -> wait for running -> /__arca/ttyd reachability`.

## Notes

- Use `PLAYWRIGHT_BASE_URL` to point tests at a different server URL.
- Keep `ARCA_SKIP_CLOUDFLARE_VALIDATION` unset unless you need to override automatic behavior.
