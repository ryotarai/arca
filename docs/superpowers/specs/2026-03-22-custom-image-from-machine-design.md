# Custom Image from Machine

Create custom images from running machines, allowing users to snapshot their configured environment and reuse it as a base for new machines.

## Use Cases

- Users template their configured environment for reuse or sharing with team members.
- Admins build golden images with organization-standard tooling from a base machine.

## Scope

- LXD and GCE runtimes (Libvirt deferred).

## Architecture Overview

### Flow

```
User clicks "Create Image" on machine detail page
  → API: CreateImageFromMachine(machine_id, name, description)
  → Validate: machine.status=running, desired_status=running, user is admin/owner
  → Set machine.desired_status = imaging
  → Enqueue create_image job (metadata: image_name; description column)
  → Return job acknowledgement

Worker picks up create_image job:
  1. Set machine.status = imaging
  2. Call arcad POST /api/prepare-for-image (synchronous cleanup)
  3. Stop machine via runtime
  4. Take snapshot via runtime (CreateImage)
  5. Create custom_images record with snapshot reference data
  6. Update job metadata with custom_image_id
  7. Restart machine via runtime
  8. Set machine.desired_status = running

On failure at any step:
  - Attempt machine restart (desired_status = running)
  - Log error to machine_events
  - Do not create custom_images record
```

### Retry After Crash

If the worker crashes mid-job, the job lease expires and another worker picks it up. On retry:

- If the machine is already stopped (step 2 already ran and arcad is unreachable), skip steps 1-2 and proceed from step 3 (stop is idempotent) or step 4.
- If the image already exists in the provider (step 4 already completed), skip creation and return existing reference data.
- Each step is designed to be safe to re-execute or skip based on current state.

### Timeouts

- Overall imaging job TTL: 15 minutes (covers arcad cleanup + stop + snapshot + restart).
- Per-step context timeouts: arcad prepare-for-image 60s, stop 90s, snapshot 10m (GCE image creation can take several minutes), restart 4m.

## Exclusion Control

- `desired_status = imaging` blocks all other operations (start, stop, delete) at the API level.
- UI disables action buttons while machine is in imaging state.
- API rejects requests if machine is not `status=running, desired_status=running`.
- API rejects if an existing `create_image` job is already pending/running for the machine.

## arcad prepare-for-image API

New synchronous HTTP endpoint on arcad:

```
POST /api/prepare-for-image
Authorization: Bearer <machine_token>
Response: 200 OK on success
```

Cleanup steps (all completed before responding):

1. Stop arca services (shelley, ttyd).
2. Remove `/etc/arca/arcad.env` (tokens, machine ID, control plane URL).
3. Remove arcad state files.
4. Run `cloud-init clean` (clears `/var/lib/cloud/`, enables re-run on next boot).
5. Remove SSH host keys (`/etc/ssh/ssh_host_*`).
6. Truncate `/etc/machine-id` (regenerated on boot).
7. Clear `arcauser` shell history (`.bash_history`, `.zsh_history`).

The arcad binary itself is left in place. On new machine boot, cloud-init re-downloads the latest version, overwriting it.

### Backward Compatibility

Older arcad versions will not have the `/api/prepare-for-image` endpoint. If the server receives a 404 response, the job fails with a clear error message indicating that the arcad version does not support image creation. The user must update arcad on the machine before retrying. Proceeding without cleanup would leave sensitive data (tokens, keys) in the image.

## Recovery After Imaging

After snapshot, the original machine is restarted. Because:

- Cloud-init userdata is stored externally (LXD: `lxc config`, GCE: instance metadata).
- Internal cloud-init state was cleared, so cloud-init re-runs on boot.
- arcad is re-downloaded and re-provisioned with the machine's original token.

The idempotent arcad setup ensures the machine fully recovers. Note that recovery involves full re-provisioning (arcad download, Ansible setup), so there is a delay before the machine is ready again.

## Runtime Snapshot Implementation

### Runtime Interface Extension

```go
type Runtime interface {
    // existing methods...
    CreateImage(ctx context.Context, machine db.Machine, imageName string) (imageRef map[string]string, error)
}
```

`CreateImage` is called on an already-stopped machine. Returns provider-specific reference data stored in `custom_images.data_json`.

### LXD

Uses `lxc` CLI (consistent with existing codebase):

```
lxc publish {containerName} --alias {imageName}
```

Returned data:
```json
{"image_alias": "{imageName}", "image_fingerprint": "{fingerprint}"}
```

### GCE

Uses `cloud.google.com/go/compute/apiv1` SDK (`ImagesClient`):

- `ImagesClient.Insert()` to create image from instance boot disk.
- Wait for operation completion via `ImagesClient.Get()` or operation polling.

Returned data:
```json
{"image_project": "{project}", "image_name": "{imageName}"}
```

GCE image resolution: `resolveImage()` is extended to support `custom_image_image_name`. When set, the image is resolved as `projects/{project}/global/images/{name}` (specific image), as opposed to the existing `image_family` path which resolves as `projects/{project}/global/images/family/{family}` (latest in family).

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

### CustomImage Message Extension

```protobuf
message CustomImage {
  // existing fields...
  string source_machine_id = 8;
}
```

### Machine Status Extension

`imaging` added to both `status` and `desired_status` enums.

## DB Schema Changes

### custom_images

```sql
ALTER TABLE custom_images ADD COLUMN source_machine_id TEXT;
```

`source_machine_id` is a soft reference (no FK constraint). If the source machine is later deleted, the image remains and the UI shows the machine as unavailable.

### machine_jobs

```sql
ALTER TABLE machine_jobs ADD COLUMN description TEXT;
ALTER TABLE machine_jobs ADD COLUMN metadata_json TEXT;
```

- `description`: generic job description column.
- `metadata_json`: job-type-specific data. For `create_image`: `{"image_name": "..."}`. Updated with `{"image_name": "...", "custom_image_id": "..."}` on success.

### machine_states

CHECK constraints on `status` and `desired_status` must be updated to include `imaging`:

```sql
-- status CHECK: add 'imaging' to allowed values
-- desired_status CHECK: add 'imaging' to allowed values
```

SQLite does not support `ALTER CONSTRAINT`. Migration must recreate the table or use the pragma-based approach consistent with the existing migration mechanism.

### machine_jobs

CHECK constraint on `kind` must be updated to include `create_image`:

```sql
-- kind CHECK: add 'create_image' to allowed values
```

Same migration approach as above.

### Migration

Migrations must be provided for both SQLite and PostgreSQL, following the existing migration mechanism in `internal/db/`.

## UI

### Machine Detail Page

- "Create Image" button, visible when user is admin/owner.
- Enabled only when `machine.status = running` and `machine.desired_status = running`.
- On click: dialog with image name (default: `{machine_name}-image-{YYYYMMDD-HHmmss}`) and optional description.
- On confirm: calls `CreateImageFromMachine`, machine transitions to imaging state.

### Imaging State Display

- Machine status shows "Creating Image..." indicator.
- Start/stop/delete buttons disabled.
- Machine events show step-by-step progress.

### Completion

- Machine returns to `running`.
- Success: toast notification "Image '{name}' created successfully".
- Failure: error toast + details in machine events.

### Custom Images List

- Images with `source_machine_id` show a link to the source machine.
- No other changes to existing image management UI.

## Template Association

When a custom image is created from a machine, it is automatically associated with the machine's template (via `template_custom_images` junction table). The `template_type` for the new custom image is derived from `machine.template_type`. This makes the image immediately available when creating new machines from the same template.

## Machine Events

The following event types are emitted during the imaging flow for progress visibility:

- `imaging_started`: job processing begins.
- `imaging_prepared`: arcad cleanup completed.
- `imaging_stopped`: machine stopped.
- `imaging_snapshot_created`: snapshot taken successfully.
- `imaging_restarting`: machine restart initiated.
- `imaging_completed`: image created and machine recovered.
- `imaging_failed`: error at any step (includes error details).
