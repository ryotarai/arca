#!/bin/sh
set -eu

if [ -z "${ARCA_TUNNEL_TOKEN:-}" ]; then
  echo "ARCA_TUNNEL_TOKEN is required" >&2
  exit 1
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

busybox httpd -f -p 8080 -h /home/arca/www &
app_pid=$!

cloudflared tunnel run --token "${ARCA_TUNNEL_TOKEN}" &
cf_pid=$!

cleanup() {
  kill "$cf_pid" "$app_pid" 2>/dev/null || true
}

trap cleanup TERM INT

wait "$cf_pid"
status=$?
cleanup
exit "$status"
