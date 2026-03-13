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
| **cloud-init** | Machine-specific env vars, arcad binary download, service start | Per-machine at first boot |
| **arcad setup** | Idempotent provisioning: packages, users, services, dev tools, SSH keys | Every arcad startup |

### Key Principles

- **cloud-init is minimal**: write `/etc/arca/arcad.env`, download arcad, start arcad systemd service. Nothing else.
- **arcad owns all setup logic**: arcad runs an idempotent provisioning phase on every startup. This makes setup logic updatable with the arcad binary.
- **All setup steps are idempotent**: each step checks current state and skips if already satisfied. Safe to re-run from any starting point (bare OS, platform image, custom image).
- **Images are a speed optimization, not a correctness requirement**: arcad's idempotent setup guarantees correctness regardless of image age.
- **Backward compatibility**: arca server must work with older arcad versions. New API fields are additive and optional.

## arcad Provisioning Phase

On every startup, arcad runs its provisioning phase before entering normal operation:

1. Install system packages (apt)
2. Create users and groups (arca, arcad, arcauser)
3. Configure sudoers
4. Create /workspace
5. Download and configure cloudflared
6. Write systemd unit files (ttyd, shelley)
7. Deploy SSH keys
8. Install dev tools (Homebrew, Claude Code, etc.)
9. Start dependent services
10. Report ready to server

Each step is idempotent: "ensure X exists / is configured" rather than "create X". Re-running the full sequence on an already-provisioned machine completes in seconds.

## arcad Update Flow

### When updates happen

arcad updates only on **machine restart** (user-initiated stop then start). arcad does **not** auto-update while running.

### Distinguishing OS reboot from arcad process restart

arcad compares `/proc/sys/kernel/random/boot_id` (unique per OS boot) against a saved value in `/var/lib/arca/last_boot_id`:

- **Different**: OS was rebooted (machine restart). Enter update phase.
- **Same**: arcad process was restarted (e.g., crash recovery). Skip update phase.

### Update sequence

```
OS boot
  |
  v
systemd starts arcad
  |
  v
arcad reads /proc/sys/kernel/random/boot_id
  |
  +-- matches saved boot_id --> skip update
  |
  +-- differs (or no saved boot_id) --> check for update
        |
        v
      GET /arcad/version (lightweight, returns version string)
        |
        +-- same version --> save boot_id, continue
        |
        +-- different version:
              |
              v
            GET /arcad/download?os=linux&arch=amd64
              |
              v
            Replace /usr/local/bin/arcad
              |
              v
            Save boot_id to /var/lib/arca/last_boot_id
              |
              v
            systemctl restart arca-arcad
              |
              v
            New arcad starts --> boot_id matches --> skip update
  |
  v
Run idempotent setup (every startup, regardless of update)
  |
  v
Report ready (includes arcad_version)
```

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

1. Start a temporary machine from the base OS image (e.g., `ubuntu:24.04`).
2. Run full arcad provisioning (cloud-init + arcad setup).
3. Wait for setup completion.
4. Snapshot the machine as an image:
   - **LXD**: `lxc publish <container> --alias arca-platform-<version>`
   - **GCE**: create disk snapshot, then `gcloud compute images create`
   - **Libvirt**: snapshot qcow2 backing file
5. Register the image in the `images` table.
6. Delete the temporary machine.

### Usage

- Each runtime has a `default_image_id` referencing the current platform image.
- `CreateMachine` uses the runtime's default image if available, falls back to bare OS if not.
- arcad's idempotent setup runs regardless, applying any steps not yet in the image.

## Scenarios

### New machine (no image)

Cloud-init writes env, downloads arcad, starts service. arcad runs full setup from scratch. Slow but always works as a fallback.

### New machine (with platform image)

Cloud-init writes env, downloads latest arcad (overwrites image-bundled version), starts service. arcad runs setup; most steps skip because image already has them. Fast.

### Running machine, server upgraded

No immediate effect. arcad continues running the old version. On next user-initiated restart, arcad updates and re-runs setup.

### Stopped machine, server upgraded, then started

OS boots, arcad starts (old version), detects new boot_id, checks `/arcad/version`, downloads new binary, restarts itself. New arcad runs setup (applies any new steps). Ready.

### Setup logic change (new package, new service)

Shipped with new arcad binary. New machines get it immediately. Existing machines get it on next restart when arcad updates. arcad's idempotent setup installs the missing package.

### Platform image rebuild

Triggered manually or as part of a release. New machines use the new image. Existing machines are unaffected (arcad update handles the delta).

### Setup failure mid-way

arcad retries with exponential backoff. Since all steps are idempotent, partial progress is safe. arcad reports error reason to server; UI shows provisioning status.

### arcad process crash (not OS reboot)

systemd restarts arcad. boot_id matches saved value, so update phase is skipped. Setup runs (idempotent, fast). Normal operation resumes.

## Future Work (Out of Scope)

- **User custom images**: snapshot-based custom images for user-specific setups. Separate design.
- **Setup progress UI**: arcad reports per-step progress events for UI display.
- **Image lifecycle management**: garbage collection of old images, retention policies.
