#!/usr/bin/env bash
# Provision an Arca machine image.
# This script pre-installs the packages and configuration that arcad's Ansible
# playbook would otherwise apply on first boot, significantly reducing machine
# startup time. arcad's setup is idempotent, so running it again on a
# pre-provisioned image is safe (already-satisfied steps are skipped).
#
# NOTE: The arcad binary itself is NOT baked into the image. It is downloaded
# via cloud-init on every boot so that machines always run the latest version.

set -euo pipefail

#--- 1. System packages (packages role) ---
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y \
  bash \
  ca-certificates \
  curl \
  git \
  jq \
  python3 \
  tmux \
  ttyd \
  build-essential \
  sudo \
  ansible

#--- 2. Users and groups (users role) ---
groupadd --system arca || true
useradd --system --gid arca --shell /usr/sbin/nologin --home /nonexistent arcad || true
useradd --gid arca --shell /bin/bash --create-home --home-dir /home/arcauser arcauser || true

#--- 3. Directories (directories role) ---
mkdir -p /workspace /etc/arca /opt/arca /var/lib/arca
chown arcauser:arca /workspace
chmod 0755 /workspace
chmod 0755 /etc/arca /opt/arca /var/lib/arca

#--- 4. Sudoers (sudoers role) ---
cat > /etc/sudoers.d/90-arcauser <<'SUDOERS'
arcauser ALL=(ALL) NOPASSWD:ALL
SUDOERS
chmod 0440 /etc/sudoers.d/90-arcauser

#--- 5. Provisioning marker ---
touch /var/lib/arca/provisioned

#--- 6. Cleanup ---
apt-get clean
rm -rf /var/lib/apt/lists/*

echo "Arca image provisioning complete."
