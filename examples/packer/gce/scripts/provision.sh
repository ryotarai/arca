#!/usr/bin/env bash
# Provision an Arca machine image using arcad --setup-once.
#
# This script downloads the arcad binary from a running Arca server and runs
# its offline setup mode, which installs system packages and configures the
# environment via the embedded Ansible playbook. This significantly reduces
# machine startup time because provisioning steps are already applied.
#
# arcad's setup is idempotent, so running it again on a pre-provisioned image
# is safe (already-satisfied steps are skipped).
#
# NOTE: The arcad binary is NOT kept in the image. It is downloaded via
# cloud-init on every boot so that machines always run the latest version.
#
# Required environment variables:
#   ARCA_SERVER_URL  - Base URL of the Arca server (e.g., http://arca.example.com:8080)
#   ARCA_API_TOKEN   - API token for authenticating with the Arca server

set -euo pipefail

: "${ARCA_SERVER_URL:?ARCA_SERVER_URL is required}"
: "${ARCA_API_TOKEN:?ARCA_API_TOKEN is required}"

export DEBIAN_FRONTEND=noninteractive

#--- 1. Install minimal dependencies for arcad download ---
apt-get update
apt-get install -y ca-certificates curl
apt-get clean
rm -rf /var/lib/apt/lists/*

#--- 2. Download arcad binary ---
arch="$(dpkg --print-architecture)"
case "$arch" in
  amd64) goarch="amd64" ;;
  arm64) goarch="arm64" ;;
  *) echo "unsupported architecture: $arch"; exit 1 ;;
esac

echo "Downloading arcad from ${ARCA_SERVER_URL}..."
curl -fsSL \
  -H "Authorization: Bearer ${ARCA_API_TOKEN}" \
  "${ARCA_SERVER_URL}/arcad/download?os=linux&arch=${goarch}" \
  -o /usr/local/bin/arcad
chmod +x /usr/local/bin/arcad

#--- 3. Run arcad --setup-once ---
echo "Running arcad --setup-once..."
/usr/local/bin/arcad --setup-once

#--- 4. Cleanup ---
rm -f /usr/local/bin/arcad

echo "Arca image provisioning complete."
