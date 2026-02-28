#!/usr/bin/env bash
set -euo pipefail

if [ -z "${ARCA_TUNNEL_TOKEN:-}" ] && [ -z "${ARCAD_TUNNEL_TOKEN:-}" ]; then
  echo "ARCA_TUNNEL_TOKEN or ARCAD_TUNNEL_TOKEN is required" >&2
  exit 1
fi
if [ -z "${ARCAD_CONTROL_PLANE_URL:-}" ]; then
  echo "ARCAD_CONTROL_PLANE_URL is required" >&2
  exit 1
fi
if [ -z "${ARCAD_MACHINE_ID:-}" ]; then
  echo "ARCAD_MACHINE_ID is required" >&2
  exit 1
fi

if [ -z "${ARCAD_TUNNEL_TOKEN:-}" ]; then
  export ARCAD_TUNNEL_TOKEN="${ARCA_TUNNEL_TOKEN}"
fi

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

/usr/local/bin/arcad &
arcad_pid=$!

BASE_PATH="${BASE_PATH:-/__arca/claudecodeui}"
PORT="${PORT:-21031}"
BASE_PATH="$BASE_PATH" PORT="$PORT" node /home/arca/claudecodeui/server/index.js &
claudecodeui_pid=$!

cleanup() {
  kill "$arcad_pid" "$claudecodeui_pid" "$app_pid" 2>/dev/null || true
}

trap cleanup TERM INT

wait -n "$arcad_pid" "$claudecodeui_pid" "$app_pid"
status=$?
cleanup
exit "$status"
