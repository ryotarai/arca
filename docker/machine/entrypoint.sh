#!/usr/bin/env bash
set -euo pipefail

if [ -z "${ARCA_TUNNEL_TOKEN:-}" ]; then
  echo "ARCA_TUNNEL_TOKEN is required" >&2
  exit 1
fi

umask 077
token_file="$(mktemp /tmp/arca-tunnel-token.XXXXXX)"
printf '%s' "${ARCA_TUNNEL_TOKEN}" > "${token_file}"
unset ARCA_TUNNEL_TOKEN

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

python3 -m http.server 8080 --directory /home/arca/www --bind 127.0.0.1 &
app_pid=$!

cloudflared tunnel run --token-file "${token_file}" &
cf_pid=$!

cleanup() {
  rm -f "${token_file}"
  kill "$cf_pid" "$app_pid" 2>/dev/null || true
}

trap cleanup TERM INT

wait "$cf_pid"
status=$?
cleanup
exit "$status"
