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

git config --global user.name "Arca"
git config --global user.email "arca@ryotarai.dev"

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

TTYD_PORT="${TTYD_PORT:-21032}"
TTYD_BASE_PATH="${TTYD_BASE_PATH:-/__arca/ttyd}"
ttyd -W -p "$TTYD_PORT" -b "$TTYD_BASE_PATH" bash &
ttyd_pid=$!

BASE_PATH="${BASE_PATH:-/__arca/claudecodeui}"
PORT="${PORT:-21031}"
VITE_IS_PLATFORM="${VITE_IS_PLATFORM:-true}"
BASE_PATH="$BASE_PATH" PORT="$PORT" VITE_IS_PLATFORM="$VITE_IS_PLATFORM" node /home/arca/claudecodeui/server/index.js &
claudecodeui_pid=$!

setup_claudecodeui() {
  local i
  local base_path="${BASE_PATH%/}"
  if [ -z "$base_path" ]; then
    base_path=""
  fi
  local status_url="http://localhost:${PORT}${base_path}/api/auth/status"
  local register_url="http://localhost:${PORT}${base_path}/api/auth/register"
  for i in $(seq 1 30); do
    if curl -fsS "$status_url" >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done

  if curl -s "$status_url" | jq -e '.needsSetup == true' >/dev/null; then
    echo "claudecodeui setup: registering default admin user"
    curl -s -X POST "$register_url" \
      -H "Content-Type: application/json" \
      -d '{"username":"admin","password":"password"}' >/dev/null
  else
    echo "claudecodeui setup: skipped (already initialized)"
  fi
}
setup_claudecodeui &
setup_pid=$!

cleanup() {
  kill "$setup_pid" "$arcad_pid" "$ttyd_pid" "$claudecodeui_pid" "$app_pid" 2>/dev/null || true
}

trap cleanup TERM INT

wait -n "$arcad_pid" "$ttyd_pid" "$claudecodeui_pid" "$app_pid"
status=$?
cleanup
exit "$status"
