# Custom Image from Machine

Create custom images from running machines, allowing users to snapshot their configured environment and reuse it as a base for new machines.

## Use Cases

- Users template their configured environment for reuse or sharing with team members.
- Admins build golden images with organization-standard tooling from a base machine.

## Scope

- LXD and GCE runtimes. Libvirt returns `ErrNotSupported` from `CreateImage()`.

## Architecture Overview

### Flow

```
User clicks "Create Image" on machine detail page
  → API: CreateImageFromMachine(machine_id, name, description)
  → Validate:
      - machine.status = running, desired_status = running
      - machine.locked_operation IS NULL
      - user is admin/owner of the machine
      - image name is valid (format + unique for template_type)
  → Set machine.locked_operation = 'create_image'
  → Enqueue create_image job (metadata_json: {image_name}, description column)
  → Return job_id

Worker picks up create_image job:
  1. Log imaging_started event
  2. Call arcad POST /api/prepare-for-image (synchronous cleanup)
  3. Log imaging_prepared event
  4. Stop machine via runtime.EnsureStopped()
  5. Log imaging_stopped event
  6. Take snapshot via runtime.CreateImage()
  7. Log imaging_snapshot_created event
  8. Create custom_images record + template association
  9. Update job metadata_json with custom_image_id
  10. Restart machine via runtime.EnsureRunning()
  11. Log imaging_restarting event
  12. Clear machine.locked_operation = NULL
  13. Log imaging_completed event

On failure at any step:
  - Attempt machine restart (call EnsureRunning)
  - Clear machine.locked_operation = NULL
  - Log imaging_failed event with error details
  - Do not create custom_images record (unless already created)
```

### Retry After Crash

If the worker crashes mid-job, the job lease expires and another worker retries. Each step checks current state:

- **Step 2 (arcad cleanup)**: If the machine is already stopped or arcad is unreachable, skip. Cleanup already ran or is no longer possible.
- **Step 4 (stop)**: `EnsureStopped()` is idempotent. Safe to re-call on an already-stopped machine.
- **Step 6 (snapshot)**: Check if an image with the target name already exists in the provider. If so, return existing reference data.
- **Step 8 (DB insert)**: Check if a custom_images record with the same `(name, template_type)` already exists. If so, use the existing record.
- **Step 12 (clear lock)**: Idempotent NULL assignment.

### Timeouts

- Overall imaging job TTL: 20 minutes.
- Per-step context timeouts: arcad prepare-for-image 60s, stop 90s, snapshot 10m (GCE image creation can take several minutes), restart 4m.
- Per-step timeouts are capped by remaining job TTL.

## Exclusion Control

### locked_operation Column

`machine_states.locked_operation` (nullable TEXT) acts as a machine-level operation lock.

- Set to `'create_image'` when imaging begins. Cleared to `NULL` on completion or failure.
- API handlers for `StartMachine`, `StopMachine`, `DeleteMachine` check `locked_operation IS NOT NULL` and reject with a descriptive error.
- The reconcile sweep query is updated to add `AND locked_operation IS NULL`, preventing the reconciler from interfering with locked machines. The `autoStopMachines` sweep must also skip locked machines.
- This pattern is extensible for future exclusive operations (e.g., resize, migrate).

### Additional Checks

- API rejects if machine is not `status=running, desired_status=running`.
- API rejects if an existing `create_image` job is already pending/running for the machine.

## arcad prepare-for-image API

New synchronous HTTP endpoint on arcad:

```
POST /api/prepare-for-image
Authorization: Bearer <machine_token>
Response: 200 OK on success
```

Cleanup steps (all completed before responding). Note: arcad itself continues running to complete the cleanup and return the response. It is terminated later when the server stops the machine.

1. Stop other arca services (shelley, ttyd) — arcad itself remains running.
2. Remove `/etc/arca/arcad.env` (tokens, machine ID, control plane URL).
3. Remove arcad state files.
4. Run `cloud-init clean` (clears `/var/lib/cloud/`, enables re-run on next boot).
5. Remove SSH host keys (`/etc/ssh/ssh_host_*`).
6. Truncate `/etc/machine-id` (regenerated on boot).
7. Clear `arcauser` shell history (`.bash_history`, `.zsh_history`).

The arcad binary itself is left in place. On new machine boot, cloud-init re-downloads the latest version, overwriting it.

### Cleanup Scope

This is the minimum required cleanup for arca-specific security (tokens, keys, machine identity). It does not perform general system sanitization (e.g., `/var/log/`, cached credentials from user-installed tools). Users should perform any additional cleanup before initiating image creation.

### Backward Compatibility

Older arcad versions will not have the `/api/prepare-for-image` endpoint. If the server receives a 404 response, the job fails with a clear error message indicating that the arcad version does not support image creation. Proceeding without cleanup would leave sensitive data (tokens, keys) in the image.

## Server-to-Arcad Communication

The worker calls arcad's HTTP API inside the machine. This is a new communication direction (normally arcad calls the server).

- **Machine IP**: obtained via `runtime.GetMachineInfo()` (LXD: `lxc list --format=json`, GCE: instance metadata).
- **Port**: arcad listens on port 21032 (existing internal API port used for readiness checks).
- **Auth**: `Authorization: Bearer <machine_token>` (token stored in the machines DB table).
- **Network**: direct connection over the private network (LXD bridge `10.200.0.0/24`, GCE VPC).
- **TLS**: plain HTTP (arcad's internal API is not TLS-enabled; traffic stays on private network).

## Recovery After Imaging

After snapshot, the original machine is restarted. Because:

- Cloud-init userdata is stored externally (LXD: `lxc config`, GCE: instance metadata).
- Internal cloud-init state was cleared, so cloud-init re-runs on boot.
- arcad is re-downloaded and re-provisioned with the machine's original token.

The idempotent arcad setup ensures the machine fully recovers. Recovery involves full re-provisioning (arcad download, Ansible setup), so there is a delay before the machine is ready again.

## Runtime Snapshot Implementation

### Runtime Interface Extension

```go
type Runtime interface {
    // existing methods...
    CreateImage(ctx context.Context, machine db.Machine, imageName string) (imageRef map[string]string, error)
}
```

`CreateImage` is called on an already-stopped machine. Returns provider-specific reference data stored in `custom_images.data_json`. The Libvirt implementation returns `ErrNotSupported`.

### LXD

Uses `lxc` CLI (consistent with existing codebase):

```
lxc publish {containerName} --alias {imageName}
lxc image info {imageName} --format=json   # to reliably retrieve fingerprint
```

Returned data:
```json
{"image_alias": "{imageName}", "image_fingerprint": "{fingerprint}"}
```

The fingerprint is retrieved via `lxc image info --format=json` after publish (more reliable than parsing `lxc publish` output across LXD versions).

### GCE

Uses `cloud.google.com/go/compute/apiv1` SDK:

1. `InstancesClient.Get()` to retrieve the instance and identify its boot disk.
2. `ImagesClient.Insert()` with `sourceDisk` set to the boot disk self-link.
3. Poll the operation until completion.

Returned data:
```json
{"image_project": "{project}", "image_name": "{imageName}"}
```

GCE image resolution: `resolveImage()` is extended to return a full source image URL. When `custom_image_image_name` is set, the URL is formatted as `projects/{project}/global/images/{name}` (specific image). The existing `image_family` path formats as `projects/{project}/global/images/family/{family}` (latest in family). The `instanceSpec()` function must use the returned URL directly rather than assuming the family format.

### Idempotency

On retry, `CreateImage` checks if an image with the target name already exists in the provider. If so, returns its reference data without re-creating.

## API Changes

### New RPC on MachineService

```protobuf
rpc CreateImageFromMachine(CreateImageFromMachineRequest) returns (CreateImageFromMachineResponse);

message CreateImageFromMachineRequest {
  string machine_id = 1;
  string name = 2;
  string description = 3;
}

message CreateImageFromMachineResponse {
  string job_id = 1;
}
```

Placed on MachineService because the operation originates from a machine.

### Image Name Validation

The API validates the image name:

- Must match `[a-z]([-a-z0-9]*[a-z0-9])?` (GCE-compatible format, applied uniformly).
- Must be unique for the machine's `template_type` in `custom_images` (pre-check at API level; DB UNIQUE constraint on `(name, template_type)` as safety net).

### Machine Message Extension

```protobuf
message Machine {
  // existing fields...
  string locked_operation = N; // "create_image" or empty
}
```

Exposed so the UI can display the imaging state and disable conflicting actions.

### CustomImage Message Extension

```protobuf
message CustomImage {
  // existing fields...
  string source_machine_id = 8;
}
```

## DB Schema Changes

### custom_images

```sql
ALTER TABLE custom_images ADD COLUMN source_machine_id TEXT;
```

Soft reference (no FK constraint). If the source machine is later deleted, the image remains and the UI shows the source machine as unavailable.

### machine_states

```sql
ALTER TABLE machine_states ADD COLUMN locked_operation TEXT;
```

No CHECK constraint changes needed. The `status` and `desired_status` columns are unchanged — the machine's real status (running/stopping/stopped/starting) is tracked normally during imaging.

### machine_jobs

```sql
ALTER TABLE machine_jobs ADD COLUMN description TEXT;
ALTER TABLE machine_jobs ADD COLUMN metadata_json TEXT;
```

- `description`: generic job description column.
- `metadata_json`: job-type-specific data. For `create_image`: `{"image_name": "..."}`. Updated with `{"image_name": "...", "custom_image_id": "..."}` on success.

The `MachineJob` Go struct and `ClaimNextMachineJob` query must be extended to include `description` and `metadata_json` fields so the worker can access them.

The CHECK constraint on `kind` must be updated to include `'create_image'`. SQLite does not support `ALTER CONSTRAINT`, so the migration must recreate the `machine_jobs` table (create new table → copy data → drop old → rename). This is a distinct migration task requiring careful testing.

### Reconcile Query Update

The reconcile sweep query (`ListMachinesByDesiredStatus` or equivalent) must be updated to exclude machines with `locked_operation IS NOT NULL`.

### Migration

Migrations must be provided for both SQLite and PostgreSQL, following the existing migration mechanism in `internal/db/`.

## UI

### Machine Detail Page

- "Create Image" button, visible when user is admin/owner.
- Enabled only when `machine.status = running`, `machine.desired_status = running`, and `machine.locked_operation` is empty.
- On click: dialog with image name (default: `{machine_name}-image-{YYYYMMDD-HHmmss}`, sanitized to GCE-compatible format) and optional description.
- On confirm: calls `CreateImageFromMachine`, machine enters locked state.

### Imaging State Display

- Machine status shows "Creating Image..." overlay alongside the real machine status (e.g., "Creating Image... (Stopping)").
- Start/stop/delete buttons disabled.
- Machine events show step-by-step progress.

### Completion

- `locked_operation` clears and machine returns to normal display.
- Success: toast notification "Image '{name}' created successfully".
- Failure: error toast + details in machine events.

### Custom Images List

- Images with `source_machine_id` show a link to the source machine (or "deleted" if machine no longer exists).
- No other changes to existing image management UI.

## Template Association

When a custom image is created from a machine, it is automatically associated with the machine's template (via `template_custom_images` junction table). The `template_type` for the new custom image is derived from `machine.template_type`. This makes the image immediately available when creating new machines from the same template. Admins can add additional template associations later via the existing image management UI.

## Machine Events

The following event types are emitted during the imaging flow for progress visibility:

- `imaging_started`: job processing begins.
- `imaging_prepared`: arcad cleanup completed.
- `imaging_stopped`: machine stopped.
- `imaging_snapshot_created`: snapshot taken successfully.
- `imaging_restarting`: machine restart initiated.
- `imaging_completed`: image created and machine recovered.
- `imaging_failed`: error at any step (includes error details).

## Out of Scope

- **Libvirt runtime**: returns `ErrNotSupported`. Can be added later.
- **Provider-level image deletion**: when a custom_images record is deleted via the existing API, the underlying LXD/GCE image is not cleaned up. Provider-side cleanup is a future enhancement.
- **General system sanitization**: the arcad cleanup covers arca-specific security requirements only. Full system sanitization (logs, cached credentials from user tools) is the user's responsibility before image creation.
