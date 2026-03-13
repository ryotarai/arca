# Machine Images and Updates

## Problem

Every machine currently boots from a bare Ubuntu 24.04 image and runs a full cloud-init setup (apt packages, user/group creation, Homebrew, dev tools, arcad download, etc.). This causes:

- **Slow startup**: several minutes for a new machine to become ready.
- **No update path**: cloud-init only runs on first boot. There is no mechanism to update arcad, packages, or setup logic on running or stopped machines.
- **Fragile provisioning**: all setup logic lives in cloud-init templates, which are hard to test and version independently.

## Design Overview

Split responsibilities into three layers:

| Layer | Responsibility | Update mechanism |
|-------|---------------|-----------------|
| **Platform Image** | Pre-installed packages, users, tools | Image rebuild |
| **cloud-init** | Machine-specific env vars, arcad binary download, service start, shutdown (image build) | Per-machine at first boot |
| **arcad setup** | Idempotent provisioning: packages, users, services, dev tools, SSH keys | Every arcad startup |

### Key Principles

- **cloud-init is minimal**: write `/etc/arca/arcad.env`, download arcad, start arcad systemd service. Nothing else.
- **arcad owns all setup logic**: arcad runs an idempotent provisioning phase on every startup. This makes setup logic updatable with the arcad binary.
- **All setup steps are idempotent**: each step checks current state and skips if already satisfied. Safe to re-run from any starting point (bare OS, platform image, custom image).
- **Images are a speed optimization, not a correctness requirement**: arcad's idempotent setup guarantees correctness regardless of image age.
- **Backward compatibility**: arca server must work with older arcad versions. New API fields are additive and optional.

## arcad Process Architecture

arcad runs as two separate processes to isolate privileges:

| Process | User | Responsibility |
|---------|------|---------------|
| `arcad` | root | Self-update, idempotent setup (apt, user creation, systemd units, etc.), service management |
| `arcad --user` | arcauser | Reverse proxy, HTTP traffic handling |

The root process performs privileged operations (package installation, user/group management, writing to `/etc/`, `systemctl` commands) and then starts the user-mode process. The user-mode process handles inbound traffic with minimal privileges.

Both are managed as separate systemd services:
- `arca-arcad.service` — root process
- `arca-arcad-user.service` — user-mode process, started by the root process after setup completes

## arcad Provisioning Phase

On every startup, the root arcad process runs its provisioning phase before entering normal operation:

1. Self-update check (first operation, see [Update Flow](#arcad-update-flow))
2. Install system packages (apt)
3. Create users and groups (arca, arcad, arcauser)
4. Configure sudoers
5. Create /workspace
6. Download and configure cloudflared
7. Write systemd unit files (ttyd, shelley, arcad-user)
8. Deploy SSH keys
9. Install dev tools (Homebrew, Claude Code, etc.)
10. Start dependent services (including `arcad --user`)
11. Report ready to server

Each step is idempotent: "ensure X exists / is configured" rather than "create X". Re-running the full sequence on an already-provisioned machine completes in seconds.

## arcad Update Flow

### When updates happen

arcad updates only on **machine restart** (user-initiated stop then start). arcad does **not** auto-update while running.

### Distinguishing machine restart from arcad process restart

arcad checks for a marker file at `/run/arca/update-checked`. Since `/run` is a tmpfs, it is cleared on every OS/container boot (including LXD container stop/start), making this approach work uniformly across all runtime types (LXD containers, libvirt VMs, GCE instances).

- **Marker absent**: machine was restarted. Enter update phase.
- **Marker present**: arcad process was restarted (e.g., crash recovery). Skip update phase.

Note: `/proc/sys/kernel/random/boot_id` is not used because it reflects the host kernel's boot ID and does not change on LXD container restart.

### Update sequence

Self-update is the **first operation** arcad performs on startup, before any setup steps. This ensures setup logic always runs at the latest version.

```
OS / container boot
  |
  v
systemd starts arcad (root)
  |
  v
arcad checks /run/arca/update-checked
  |
  +-- exists --> skip update
  |
  +-- absent --> check for update
        |
        v
      GET /arcad/version (lightweight, returns version string)
        |
        +-- same version --> write marker, continue
        |
        +-- different version (or local version is "dev"):
              |
              v
            GET /arcad/download?os=linux&arch=amd64
              |
              v
            Write to temp file → rename to /usr/local/bin/arcad (atomic)
              |
              v
            Write /run/arca/update-checked marker
              |
              v
            systemctl restart arca-arcad
              |
              v
            New arcad starts --> marker exists --> skip update
  |
  v
Run idempotent setup (every startup, regardless of update)
  |
  v
Report ready (includes arcad_version)
```

### Binary replacement

Binary replacement must be **atomic**: write the new binary to a temporary file in the same filesystem, then `rename()` to `/usr/local/bin/arcad`. This prevents corruption if the process is interrupted during the write.

### Versioning

Both arca server and arcad use the same version string, set via Go linker flags (`-ldflags -X ...`) at build time. The `/arcad/version` endpoint returns the version of the arcad binary the server would serve.

When the local version is `(devel)` (local development build), arcad **always** downloads from the server, since it cannot meaningfully compare versions.

## Version Reporting

arcad reports its version to the server via the existing `ReportMachineReadiness` RPC:

```protobuf
message ReportMachineReadinessRequest {
  bool ready = 1;
  string reason = 2;
  string machine_id = 3;
  string container_id = 4;
  string arcad_version = 5;  // new field
}
```

The server stores the version in `machine_states.arcad_version` and can:

- Display arcad version in the machine detail UI.
- Show an "update available" indicator when the reported version differs from the server's current version.
- Provide an admin overview of arcad versions across all machines.

## Server API Changes

### New endpoint: `GET /arcad/version`

Lightweight endpoint for arcad to check whether an update is available without downloading the full binary.

- **Request**: `GET /arcad/version?os={os}&arch={arch}` with machine token auth.
- **Response**: version string (e.g., `v0.1.42`).
- arcad compares against its own compiled-in version to decide whether to download.

### Modified endpoint: `ReportMachineReadiness`

Add optional `arcad_version` field (field number 5). Older arcad instances that don't send this field continue to work (empty string, server ignores).

## Platform Images

### Purpose

Pre-built images containing packages, users, tools, and an arcad binary. Machines starting from a platform image skip most of the provisioning phase (steps already satisfied), reducing startup to seconds.

### Scope

Images are **per-runtime**. Each runtime (LXD host, GCE project, libvirt host) manages its own images because images are not portable across runtime types or hosts.

```sql
CREATE TABLE images (
    id TEXT PRIMARY KEY,
    runtime_id TEXT NOT NULL REFERENCES runtimes(id),
    name TEXT NOT NULL,
    version TEXT,
    type TEXT NOT NULL,        -- 'platform' or 'custom'
    status TEXT NOT NULL,      -- 'building', 'ready', 'failed'
    source_machine_id TEXT,    -- for custom images: the machine snapshotted
    platform_ref TEXT,         -- runtime-specific reference (LXD alias, GCE image name, etc.)
    created_by TEXT,
    created_at TIMESTAMP,
    UNIQUE(runtime_id, name)
);
```

### Build process

The image build uses a special arcad mode (`--mode=image-build`) that runs setup without machine-specific data. After arcad completes, cloud-init handles cleanup and shutdown.

**What the image contains after build:**
- System packages, users/groups, sudoers, /workspace
- cloudflared binary
- systemd unit files (installed but **not enabled**)
- Dev tools (Homebrew, Claude Code, etc.)
- arcad binary (build-time version; overwritten on real machine boot)

**What the image does NOT contain:**
- `/etc/arca/arcad.env` (no machine tokens, IDs, or control plane URLs)
- SSH keys
- `/run/arca/update-checked` (tmpfs, not persisted)
- Enabled systemd services (arca-arcad, arca-arcad-user, ttyd, shelley)
- cloud-init state (cleaned before snapshot)

**Build sequence:**

```
Worker creates temporary machine from bare OS (e.g., ubuntu:24.04)
  |
  v
cloud-init (image-build variant):
  1. Download arcad binary
  2. Run: arcad --mode=image-build
     (does NOT write arcad.env, does NOT enable systemd services)
  3. Wait for arcad to exit
  4. Run: cloud-init clean (remove cloud-init state so it re-runs on real boot)
  5. Initiate OS shutdown
  |
  v
arcad (image-build mode):
  1. Run idempotent setup (packages, users, tools, unit files)
  2. Skip machine-specific steps (SSH keys, env, service enable)
  3. Exit (does NOT shutdown; cloud-init handles that)
  |
  v
cloud-init continues after arcad exits:
  4. cloud-init clean
  5. shutdown
  |
  v
Worker detects machine stopped (timeout: build fails if not stopped within limit)
  |
  v
Worker snapshots the machine:
  - LXD: lxc publish <container> --alias arca-platform-<version>
  - GCE: create disk snapshot → gcloud compute images create
  - Libvirt: snapshot qcow2 backing file
  |
  v
Worker registers image in images table → deletes temporary machine
```

**Normal machine boot from platform image:**

```
cloud-init (runs fresh thanks to cloud-init clean in the image):
  1. Write /etc/arca/arcad.env (machine ID, token, control plane URL)
  2. Download latest arcad binary (overwrites image version)
  3. systemctl enable --now arca-arcad
  |
  v
arcad (normal mode, root):
  1. Self-update check (marker absent on fresh boot → check version → already latest → write marker)
  2. Run idempotent setup (most steps skip, deploys SSH keys, etc.)
  3. Start arcad --user and dependent services
  4. Report ready
```

### Usage

- Each runtime has a `default_image_id` referencing the current platform image.
- `CreateMachine` uses the runtime's default image if available, falls back to bare OS if not.
- arcad's idempotent setup runs regardless, applying any steps not yet in the image.

### Build timeout

Image builds use a generous timeout (configurable, e.g., 15–20 minutes) to account for full setup from bare OS. If the machine does not stop within the timeout, the build is marked as failed and the temporary machine is deleted.

## Scenarios

### New machine (no image)

Cloud-init writes env, downloads arcad, starts service. arcad runs full setup from scratch. Slow but always works as a fallback. Readiness timeout should be extended (e.g., 15 minutes) to accommodate full provisioning.

### New machine (with platform image)

Cloud-init writes env, downloads latest arcad (overwrites image-bundled version), starts service. arcad runs setup; most steps skip because image already has them. Fast.

### Running machine, server upgraded

No immediate effect. arcad continues running the old version. On next user-initiated restart, arcad updates and re-runs setup.

### Stopped machine, server upgraded, then started

OS boots, arcad starts (old version), no update-checked marker in /run → checks `/arcad/version`, downloads new binary, restarts itself. New arcad runs setup (applies any new steps). Ready.

### Setup logic change (new package, new service)

Shipped with new arcad binary. New machines get it immediately. Existing machines get it on next restart when arcad updates. arcad's idempotent setup installs the missing package.

### Platform image rebuild

Triggered manually by admin via UI/API. New machines use the new image. Existing machines are unaffected (arcad update handles the delta).

### Setup failure mid-way

arcad retries failed steps with exponential backoff. Since all steps are idempotent, partial progress is safe. arcad reports error reason to server; UI shows provisioning status.

### arcad process crash (not machine reboot)

systemd restarts arcad. Update-checked marker exists in /run, so update phase is skipped. Setup runs (idempotent, fast). Normal operation resumes.

## Rollback

Automatic server/arcad rollback is out of scope. arcad's idempotent setup is additive ("ensure X exists") and does not remove state created by newer versions.

If a rollback is required:

1. **Roll back arca server** to the previous version.
2. **Restart affected machines** (stop → start). arcad will download the older binary from the server and re-run setup.
3. **Verify**: check arcad version reported in the UI matches the expected version.
4. **Manual cleanup** (if needed): if a newer arcad version added files, services, or packages that the older version does not manage, remove them manually via SSH:
   ```bash
   # Example: remove a service added by a newer version
   ssh arcauser@<machine-ip> sudo systemctl disable --now <service-name>
   ssh arcauser@<machine-ip> sudo rm /etc/systemd/system/<service-name>.service
   ```
5. **Rebuild platform images** so new machines use the rolled-back version.

## Future Work (Out of Scope)

- **User custom images**: snapshot-based custom images for user-specific setups. Separate design.
- **Setup progress UI**: arcad reports per-step progress events for UI display.
- **Image lifecycle management**: garbage collection of old images, retention policies.
