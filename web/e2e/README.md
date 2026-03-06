# E2E test guide

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
