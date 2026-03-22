# Custom Image from Machine — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow users to create reusable custom images by snapshotting running machines (LXD + GCE).

**Architecture:** Job-based async flow using the existing `machine_jobs` queue. A `locked_operation` column on `machine_states` provides exclusion control. The worker orchestrates: arcad cleanup → stop → snapshot → create custom_image record → restart. The arcad guest daemon gets a new `POST /api/prepare-for-image` endpoint for secure state cleanup.

**Tech Stack:** Go 1.22, ConnectRPC/protobuf, SQLite/PostgreSQL, React + shadcn/ui, `cloud.google.com/go/compute/apiv1` (GCE SDK), `lxc` CLI (LXD)

**Spec:** `docs/superpowers/specs/2026-03-22-custom-image-from-machine-design.md`

---

## File Structure

### New Files
| File | Responsibility |
|------|---------------|
| `internal/db/migrations_v2/000046_create_image_from_machine.up.sql` | DB migration: locked_operation, job columns, CHECK constraint |
| `internal/db/migrations_v2/000046_create_image_from_machine.down.sql` | Rollback migration |
| `internal/arcad/prepare_for_image.go` | arcad prepare-for-image HTTP handler |
| `internal/arcad/prepare_for_image_test.go` | Tests for prepare-for-image handler |
| `internal/machine/image_job.go` | Worker logic for create_image job |
| `internal/machine/image_job_test.go` | Tests for image job handler |
| `web/src/components/CreateImageDialog.tsx` | UI dialog for creating image from machine |

### Modified Files
| File | Changes |
|------|---------|
| `internal/db/sqlc/schema.sql` | Add locked_operation, job columns, update CHECK |
| `internal/db/sqlc/query.sql` | New queries: update locked_operation, create_image job, filter sweeps |
| `internal/db/machine_store.go` | MachineJob struct extension, locked_operation methods |
| `internal/machine/worker.go` | Route create_image jobs, exclude locked from sweeps |
| `internal/machine/lxd_runtime.go` | CreateImage method |
| `internal/machine/gce_runtime.go` | CreateImage method, resolveImage extension |
| `internal/machine/libvirt_runtime.go` | CreateImage stub (ErrNotSupported) |
| `internal/db/custom_image_store.go` | source_machine_id support |
| `proto/arca/v1/machine.proto` | CreateImageFromMachine RPC, locked_operation field |
| `proto/arca/v1/image.proto` | source_machine_id field |
| `internal/server/machine_connect.go` | CreateImageFromMachine handler, locked_operation checks |
| `cmd/arcad/main.go` | Register prepare-for-image handler |
| `web/src/lib/api.ts` | createImageFromMachine API function |
| `web/src/pages/MachineDetailPage.tsx` | Create Image button, imaging state display |
| `web/src/lib/types.ts` | locked_operation field on Machine type |

---

## Task 1: DB Migration — locked_operation, job columns, CHECK constraint

**Files:**
- Create: `internal/db/migrations_v2/000046_create_image_from_machine.up.sql`
- Create: `internal/db/migrations_v2/000046_create_image_from_machine.down.sql`
- Modify: `internal/db/sqlc/schema.sql`

- [ ] **Step 1: Write the up migration**

SQLite requires table recreation for CHECK constraint changes on `machine_jobs.kind`. The `machine_states.locked_operation` and `custom_images.source_machine_id` are simple ALTER TABLE additions.

```sql
-- internal/db/migrations_v2/000046_create_image_from_machine.up.sql

-- Add locked_operation to machine_states
ALTER TABLE machine_states ADD COLUMN locked_operation TEXT;

-- Add source_machine_id to custom_images
ALTER TABLE custom_images ADD COLUMN source_machine_id TEXT;

-- Recreate machine_jobs to update CHECK constraint on kind and add new columns
CREATE TABLE machine_jobs_new (
  id TEXT PRIMARY KEY,
  machine_id TEXT NOT NULL REFERENCES machines(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('start', 'stop', 'delete', 'reconcile', 'create_image')),
  status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
  attempt INTEGER NOT NULL DEFAULT 0,
  next_run_at BIGINT NOT NULL,
  lease_owner TEXT,
  lease_until BIGINT,
  last_error TEXT,
  description TEXT,
  metadata_json TEXT,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL
);

INSERT INTO machine_jobs_new (id, machine_id, kind, status, attempt, next_run_at, lease_owner, lease_until, last_error, created_at, updated_at)
SELECT id, machine_id, kind, status, attempt, next_run_at, lease_owner, lease_until, last_error, created_at, updated_at FROM machine_jobs;

DROP TABLE machine_jobs;
ALTER TABLE machine_jobs_new RENAME TO machine_jobs;
```

Note: Check the existing schema for any indexes on machine_jobs that need to be recreated after the table rename.

- [ ] **Step 2: Write the down migration**

```sql
-- internal/db/migrations_v2/000046_create_image_from_machine.down.sql

-- Remove locked_operation is not supported in SQLite (ALTER TABLE DROP COLUMN not available in older versions).
-- For rollback, recreate table without the column if needed.
-- For now, a minimal down migration:

-- Recreate machine_jobs without new columns and with old CHECK
CREATE TABLE machine_jobs_old (
  id TEXT PRIMARY KEY,
  machine_id TEXT NOT NULL REFERENCES machines(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('start', 'stop', 'delete', 'reconcile')),
  status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
  attempt INTEGER NOT NULL DEFAULT 0,
  next_run_at BIGINT NOT NULL,
  lease_owner TEXT,
  lease_until BIGINT,
  last_error TEXT,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL
);

INSERT INTO machine_jobs_old SELECT id, machine_id, kind, status, attempt, next_run_at, lease_owner, lease_until, last_error, created_at, updated_at FROM machine_jobs WHERE kind != 'create_image';

DROP TABLE machine_jobs;
ALTER TABLE machine_jobs_old RENAME TO machine_jobs;
```

- [ ] **Step 3: Update sqlc schema.sql to match**

Update `internal/db/sqlc/schema.sql`:
- Add `locked_operation TEXT` to `machine_states` table definition.
- Add `description TEXT`, `metadata_json TEXT` to `machine_jobs` table definition.
- Add `'create_image'` to `machine_jobs.kind` CHECK constraint.
- Add `source_machine_id TEXT` to `custom_images` table definition.

- [ ] **Step 4: Run `make sqlc` and verify compilation**

Run: `make sqlc && go vet ./...`

- [ ] **Step 5: Commit**

```bash
git add internal/db/migrations_v2/000046_* internal/db/sqlc/schema.sql internal/db/sqlc/sqlite/*.go internal/db/sqlc/postgresql/*.go
git commit -m "Add DB migration for create_image job and locked_operation"
```

---

## Task 2: Machine Store — MachineJob extension and locked_operation queries

**Files:**
- Modify: `internal/db/sqlc/query.sql`
- Modify: `internal/db/machine_store.go`

- [ ] **Step 1: Add sqlc queries**

Add to `internal/db/sqlc/query.sql`:

```sql
-- name: SetMachineLockedOperation :exec
UPDATE machine_states SET locked_operation = sqlc.arg(locked_operation), updated_at = sqlc.arg(now_unix) WHERE machine_id = sqlc.arg(machine_id);

-- name: ClearMachineLockedOperation :exec
UPDATE machine_states SET locked_operation = NULL, updated_at = sqlc.arg(now_unix) WHERE machine_id = sqlc.arg(machine_id);

-- name: GetMachineLockedOperation :one
SELECT locked_operation FROM machine_states WHERE machine_id = sqlc.arg(machine_id);

-- name: EnqueueMachineJobWithMeta :exec
INSERT INTO machine_jobs (
  id, machine_id, kind, status, attempt, next_run_at, description, metadata_json, created_at, updated_at
)
VALUES (
  sqlc.arg(id),
  sqlc.arg(machine_id),
  sqlc.arg(kind),
  'queued',
  0,
  sqlc.arg(next_run_at),
  sqlc.arg(description),
  sqlc.arg(metadata_json),
  sqlc.arg(now_unix),
  sqlc.arg(now_unix)
);

-- name: UpdateMachineJobMetadataJSON :exec
UPDATE machine_jobs SET metadata_json = sqlc.arg(metadata_json), updated_at = sqlc.arg(now_unix) WHERE id = sqlc.arg(id);
```

Update `ListMachinesByDesiredStatus` to exclude locked machines:

```sql
-- name: ListMachinesByDesiredStatus :many
SELECT m.id, m.name, m.template_id, m.template_type, m.template_config_json, m.setup_version, m.options_json, m.custom_image_id, ms.status, ms.desired_status, ms.container_id, ms.last_error, ms.ready, ms.ready_reported_at, ms.ready_reason, ms.arcad_version, ms.last_activity_at
FROM machines m
JOIN machine_states ms ON ms.machine_id = m.id
WHERE ms.desired_status = sqlc.arg(desired_status)
  AND ms.locked_operation IS NULL
ORDER BY ms.updated_at ASC
LIMIT sqlc.arg(limit_n);
```

- [ ] **Step 2: Run `make sqlc`**

Run: `make sqlc`

- [ ] **Step 3: Extend MachineJob struct and ClaimNextMachineJob**

In `internal/db/machine_store.go`:
- Add `Description string` and `MetadataJSON string` fields to `MachineJob` struct.
- Update `ClaimNextMachineJob` to read and populate these new fields.
- Add `MachineJobCreateImage = "create_image"` constant.
- Add `Machine.LockedOperation` field to the `Machine` struct.
- Update machine-reading queries to populate `LockedOperation`.

- [ ] **Step 4: Add store methods**

In `internal/db/machine_store.go`:

```go
func (s *Store) SetMachineLockedOperation(ctx context.Context, machineID, operation string) error
func (s *Store) ClearMachineLockedOperation(ctx context.Context, machineID string) error
func (s *Store) EnqueueCreateImageJob(ctx context.Context, machineID, description, metadataJSON string) (string, error)
func (s *Store) UpdateMachineJobMetadataJSON(ctx context.Context, jobID, metadataJSON string) error
```

- [ ] **Step 5: Run tests**

Run: `go vet ./internal/db/... && go test ./internal/db/...`

- [ ] **Step 6: Commit**

```bash
git add internal/db/sqlc/query.sql internal/db/sqlc/sqlite/*.go internal/db/sqlc/postgresql/*.go internal/db/machine_store.go
git commit -m "Add locked_operation queries and MachineJob metadata support"
```

---

## Task 3: Custom Image Store — source_machine_id support

**Files:**
- Modify: `internal/db/sqlc/query.sql`
- Modify: `internal/db/custom_image_store.go`

- [ ] **Step 1: Update sqlc queries for source_machine_id**

Add a new query or update the existing `InsertCustomImage` query in `internal/db/sqlc/query.sql` to include `source_machine_id`.

Update `ListCustomImages` and related queries to return `source_machine_id`.

- [ ] **Step 2: Run `make sqlc`**

- [ ] **Step 3: Update CustomImage struct and store methods**

In `internal/db/custom_image_store.go`:
- Add `SourceMachineID string` to the `CustomImage` struct.
- Add `CreateCustomImageFromMachine` method that accepts `sourceMachineID` and auto-associates with a template.

```go
func (s *Store) CreateCustomImageFromMachine(ctx context.Context, name, templateType, dataJSON, description, sourceMachineID, templateID string) (*CustomImage, error)
```

This method:
1. Creates the custom_images record with source_machine_id.
2. Inserts into template_custom_images to associate with the source machine's template.
3. Uses a transaction for atomicity.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/db/...`

- [ ] **Step 5: Commit**

```bash
git add internal/db/sqlc/query.sql internal/db/sqlc/sqlite/*.go internal/db/sqlc/postgresql/*.go internal/db/custom_image_store.go
git commit -m "Add source_machine_id to custom images and creation method"
```

---

## Task 4: Proto Changes — CreateImageFromMachine RPC

**Files:**
- Modify: `proto/arca/v1/machine.proto`
- Modify: `proto/arca/v1/image.proto`

- [ ] **Step 1: Update machine.proto**

Add to `MachineService`:
```protobuf
rpc CreateImageFromMachine(CreateImageFromMachineRequest) returns (CreateImageFromMachineResponse);
```

Add `locked_operation` to `Machine` message (field 17):
```protobuf
string locked_operation = 17;
```

Add request/response messages:
```protobuf
message CreateImageFromMachineRequest {
  string machine_id = 1;
  string name = 2;
  string description = 3;
}

message CreateImageFromMachineResponse {
  string job_id = 1;
}
```

- [ ] **Step 2: Update image.proto**

Add `source_machine_id` to `CustomImage` message (field 8):
```protobuf
string source_machine_id = 8;
```

- [ ] **Step 3: Regenerate proto code**

Run: `make proto`

- [ ] **Step 4: Verify compilation**

Run: `go vet ./...`

- [ ] **Step 5: Commit**

```bash
git add proto/ internal/gen/ web/src/gen/
git commit -m "Add CreateImageFromMachine RPC and locked_operation proto field"
```

---

## Task 5: Runtime Interface — CreateImage method

**Files:**
- Modify: `internal/machine/worker.go` (interface definition)
- Modify: `internal/machine/lxd_runtime.go`
- Modify: `internal/machine/gce_runtime.go`
- Modify: `internal/machine/libvirt_runtime.go`

- [ ] **Step 1: Define ErrNotSupported and extend Runtime interface**

In `internal/machine/worker.go`, add:

```go
var ErrImageCreationNotSupported = errors.New("image creation not supported for this runtime")
```

Extend the `Runtime` interface:
```go
type Runtime interface {
    // existing methods...
    CreateImage(ctx context.Context, machine db.Machine, imageName string) (map[string]string, error)
}
```

- [ ] **Step 2: Implement LXD CreateImage**

In `internal/machine/lxd_runtime.go`:

```go
func (r *LxdRuntime) CreateImage(ctx context.Context, machine db.Machine, imageName string) (map[string]string, error) {
    name := containerName(machine.ID)

    // Check if image already exists (idempotency)
    checkCmd := exec.CommandContext(ctx, "lxc", "image", "info", imageName, "--format=json")
    if checkOut, err := checkCmd.Output(); err == nil {
        // Image exists — parse fingerprint and return
        var info struct{ Fingerprint string }
        if json.Unmarshal(checkOut, &info) == nil && info.Fingerprint != "" {
            return map[string]string{"image_alias": imageName, "image_fingerprint": info.Fingerprint}, nil
        }
    }

    // Publish container as image
    publishCmd := exec.CommandContext(ctx, "lxc", "publish", name, "--alias", imageName)
    if out, err := publishCmd.CombinedOutput(); err != nil {
        return nil, fmt.Errorf("lxc publish failed: %s: %w", string(out), err)
    }

    // Retrieve fingerprint reliably
    infoCmd := exec.CommandContext(ctx, "lxc", "image", "info", imageName, "--format=json")
    infoOut, err := infoCmd.Output()
    if err != nil {
        return nil, fmt.Errorf("lxc image info failed: %w", err)
    }
    var info struct{ Fingerprint string }
    if err := json.Unmarshal(infoOut, &info); err != nil {
        return nil, fmt.Errorf("parse image info failed: %w", err)
    }

    return map[string]string{
        "image_alias":       imageName,
        "image_fingerprint": info.Fingerprint,
    }, nil
}
```

- [ ] **Step 3: Implement GCE CreateImage**

In `internal/machine/gce_runtime.go`:

Add `cloud.google.com/go/compute/apiv1` and `computepb` imports. Add a method that:
1. Gets the instance to find the boot disk name.
2. Checks if image already exists (idempotency).
3. Calls `ImagesClient.Insert()` with the boot disk as source.
4. Waits for the operation to complete.
5. Returns `{"image_project": project, "image_name": imageName}`.

Also extend `resolveImage()` to handle `custom_image_image_name`:
- If `custom_image_image_name` is set, return `projects/{project}/global/images/{name}`.
- If `custom_image_image_family` is set (existing), return `projects/{project}/global/images/family/{family}`.

Update `instanceSpec()` to use the full image URL from `resolveImage()` directly.

- [ ] **Step 4: Add Libvirt stub**

In `internal/machine/libvirt_runtime.go`:

```go
func (r *LibvirtRuntime) CreateImage(ctx context.Context, machine db.Machine, imageName string) (map[string]string, error) {
    return nil, ErrImageCreationNotSupported
}
```

- [ ] **Step 5: Verify compilation**

Run: `go vet ./internal/machine/...`

- [ ] **Step 6: Commit**

```bash
git add internal/machine/worker.go internal/machine/lxd_runtime.go internal/machine/gce_runtime.go internal/machine/libvirt_runtime.go go.mod go.sum
git commit -m "Add CreateImage to Runtime interface with LXD, GCE, and Libvirt implementations"
```

---

## Task 6: arcad — prepare-for-image endpoint

**Files:**
- Create: `internal/arcad/prepare_for_image.go`
- Create: `internal/arcad/prepare_for_image_test.go`
- Modify: `cmd/arcad/main.go`

- [ ] **Step 1: Write the prepare-for-image handler**

Create `internal/arcad/prepare_for_image.go`:

```go
package arcad

import (
    "fmt"
    "log"
    "net/http"
    "os"
    "os/exec"
    "path/filepath"
)

// PrepareForImageHandler returns an HTTP handler that cleans up
// machine-specific state in preparation for image creation.
func PrepareForImageHandler(cfg Config) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
            http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
            return
        }

        // Verify authorization
        token := r.Header.Get("Authorization")
        if token != "Bearer "+cfg.MachineToken {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }

        if err := prepareForImage(); err != nil {
            log.Printf("prepare-for-image failed: %v", err)
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }

        w.WriteHeader(http.StatusOK)
    }
}

func prepareForImage() error {
    steps := []struct {
        name string
        fn   func() error
    }{
        {"stop shelley/ttyd", stopArcaServices},
        {"remove arcad env", func() error { return os.Remove("/etc/arca/arcad.env") }},
        {"remove arcad state", removeArcadState},
        {"clean cloud-init", func() error {
            return exec.Command("cloud-init", "clean").Run()
        }},
        {"remove SSH host keys", removeSSHHostKeys},
        {"truncate machine-id", func() error { return os.WriteFile("/etc/machine-id", []byte(""), 0644) }},
        {"clear user history", clearUserHistory},
    }

    for _, step := range steps {
        log.Printf("prepare-for-image: %s", step.name)
        if err := step.fn(); err != nil {
            // Log but continue — best effort for non-critical steps
            log.Printf("prepare-for-image: %s: %v", step.name, err)
        }
    }
    return nil
}
```

Implement helper functions: `stopArcaServices` (systemctl stop shelley ttyd), `removeArcadState`, `removeSSHHostKeys` (glob `/etc/ssh/ssh_host_*`), `clearUserHistory` (remove `.bash_history`, `.zsh_history` for arcauser).

- [ ] **Step 2: Write tests**

Create `internal/arcad/prepare_for_image_test.go`:
- Test handler rejects non-POST requests.
- Test handler rejects unauthorized requests.
- Test handler accepts valid Bearer token.
- Note: actual cleanup cannot be tested in unit tests (requires root/real FS). Test the HTTP handler logic only.

- [ ] **Step 3: Register handler in cmd/arcad/main.go**

In `runUserMode`, the proxy handles all requests. The prepare-for-image endpoint needs to be added before the proxy. Wrap the proxy with a mux or add the handler to the existing proxy's routing.

In `cmd/arcad/main.go`, modify the `httpServer.Handler` setup:

```go
mux := http.NewServeMux()
mux.Handle("/api/prepare-for-image", arcad.PrepareForImageHandler(cfg))
mux.Handle("/", proxy)
httpServer.Handler = mux
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/arcad/...`

- [ ] **Step 5: Commit**

```bash
git add internal/arcad/prepare_for_image.go internal/arcad/prepare_for_image_test.go cmd/arcad/main.go
git commit -m "Add arcad prepare-for-image endpoint for image creation cleanup"
```

---

## Task 7: Worker — create_image job handler

**Files:**
- Create: `internal/machine/image_job.go`
- Create: `internal/machine/image_job_test.go`
- Modify: `internal/machine/worker.go`

- [ ] **Step 1: Add job routing in worker.go**

In `internal/machine/worker.go`, update the job dispatch switch (around line 330):

```go
case db.MachineJobCreateImage:
    err = w.handleCreateImage(ctx, machine, job)
```

- [ ] **Step 2: Write the handleCreateImage method**

Create `internal/machine/image_job.go`:

```go
package machine

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/ryotarai/arca/internal/db"
)

type createImageMetadata struct {
    ImageName      string `json:"image_name"`
    CustomImageID  string `json:"custom_image_id,omitempty"`
}

func (w *Worker) handleCreateImage(ctx context.Context, machine db.Machine, job db.MachineJob) error {
    var meta createImageMetadata
    if err := json.Unmarshal([]byte(job.MetadataJSON), &meta); err != nil {
        return fmt.Errorf("parse job metadata: %w", err)
    }

    w.emitEvent(ctx, machine.ID, job.ID, "info", "imaging_started", "Image creation started")

    // Step 1: Call arcad prepare-for-image (skip if machine already stopped)
    if machine.Status == db.MachineStatusRunning {
        if err := w.callArcadPrepareForImage(ctx, machine); err != nil {
            return fmt.Errorf("arcad prepare-for-image: %w", err)
        }
        w.emitEvent(ctx, machine.ID, job.ID, "info", "imaging_prepared", "Machine state cleaned for imaging")
    }

    // Step 2: Stop machine
    stopCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
    defer cancel()
    if err := w.runtime.EnsureStopped(stopCtx, machine); err != nil {
        return fmt.Errorf("stop machine: %w", err)
    }
    w.emitEvent(ctx, machine.ID, job.ID, "info", "imaging_stopped", "Machine stopped")

    // Step 3: Create image snapshot
    snapshotCtx, cancel2 := context.WithTimeout(ctx, 10*time.Minute)
    defer cancel2()
    imageRef, err := w.runtime.CreateImage(snapshotCtx, machine, meta.ImageName)
    if err != nil {
        return fmt.Errorf("create image: %w", err)
    }
    w.emitEvent(ctx, machine.ID, job.ID, "info", "imaging_snapshot_created", "Snapshot created")

    // Step 4: Create custom_images record (idempotent)
    dataJSON, _ := json.Marshal(imageRef)
    customImage, err := w.store.CreateCustomImageFromMachine(ctx, meta.ImageName, machine.TemplateType, string(dataJSON), job.Description, machine.ID, machine.TemplateID)
    if err != nil {
        return fmt.Errorf("create custom image record: %w", err)
    }

    // Step 5: Update job metadata with custom_image_id
    meta.CustomImageID = customImage.ID
    metaBytes, _ := json.Marshal(meta)
    _ = w.store.UpdateMachineJobMetadataJSON(ctx, job.ID, string(metaBytes))

    // Step 6: Restart machine
    w.emitEvent(ctx, machine.ID, job.ID, "info", "imaging_restarting", "Restarting machine")
    startCtx, cancel3 := context.WithTimeout(ctx, 4*time.Minute)
    defer cancel3()
    startOpts := w.buildStartOptions(machine)
    if _, err := w.runtime.EnsureRunning(startCtx, machine, startOpts); err != nil {
        // Log but don't fail the job — image was created successfully
        w.emitEvent(ctx, machine.ID, job.ID, "warn", "imaging_restart_failed", fmt.Sprintf("Failed to restart: %v", err))
    }

    // Step 7: Clear lock
    if err := w.store.ClearMachineLockedOperation(ctx, machine.ID); err != nil {
        return fmt.Errorf("clear locked_operation: %w", err)
    }

    w.emitEvent(ctx, machine.ID, job.ID, "info", "imaging_completed", fmt.Sprintf("Image '%s' created successfully", meta.ImageName))
    return nil
}
```

- [ ] **Step 3: Implement callArcadPrepareForImage helper**

In the same file, add a method that:
1. Gets machine IP via `w.runtime.GetMachineInfo(ctx, machine)`.
2. Makes `POST http://{ip}:21030/api/prepare-for-image` with `Authorization: Bearer {machine.MachineToken}`.
3. 60s timeout.
4. Returns error on non-200 response (special message for 404 — old arcad version).

Note: arcad listens on port 21030 (from `cfg.ListenAddr` default `:21030` in config.go).

- [ ] **Step 4: Update handleJobResult for create_image failure recovery**

When a create_image job fails, the worker must:
1. Attempt to restart the machine (`EnsureRunning`).
2. Clear `locked_operation`.
3. Emit `imaging_failed` event.

Modify the error handling path in the worker's main job loop to handle this for `MachineJobCreateImage` jobs.

- [ ] **Step 5: Write tests**

Create `internal/machine/image_job_test.go`:
- Test the full flow with a mock runtime and store.
- Test retry when machine is already stopped (skip arcad call).
- Test retry when image already exists.
- Test failure recovery (lock cleared, machine restarted).

- [ ] **Step 6: Run tests**

Run: `go test ./internal/machine/...`

- [ ] **Step 7: Commit**

```bash
git add internal/machine/image_job.go internal/machine/image_job_test.go internal/machine/worker.go
git commit -m "Add create_image job handler with arcad cleanup and snapshot"
```

---

## Task 8: API Handler — CreateImageFromMachine

**Files:**
- Modify: `internal/server/machine_connect.go`

- [ ] **Step 1: Add locked_operation checks to existing handlers**

In `StartMachine`, `StopMachine`, `DeleteMachine` handlers, after the role check and before the state transition, add:

```go
if machine.LockedOperation != "" {
    return nil, connect.NewError(connect.CodeFailedPrecondition,
        fmt.Errorf("machine is locked by operation: %s", machine.LockedOperation))
}
```

- [ ] **Step 2: Implement CreateImageFromMachine handler**

```go
func (s *MachineConnectHandler) CreateImageFromMachine(ctx context.Context, req *connect.Request[arcav1.CreateImageFromMachineRequest]) (*connect.Response[arcav1.CreateImageFromMachineResponse], error) {
    // 1. Validate user role (admin/owner)
    // 2. Fetch machine, validate status=running, desired_status=running, locked_operation=NULL
    // 3. Validate image name format: [a-z]([-a-z0-9]*[a-z0-9])?
    // 4. Check image name uniqueness for template_type
    // 5. Set locked_operation = 'create_image'
    // 6. Enqueue create_image job with metadata_json = {"image_name": name}
    // 7. Return job_id
}
```

- [ ] **Step 3: Add locked_operation to Machine proto conversion**

In the `machineToProto` helper function (or equivalent), populate the `locked_operation` field from `db.Machine.LockedOperation`.

- [ ] **Step 4: Add source_machine_id to CustomImage proto conversion**

In the `customImageToProto` helper (in `image_connect.go`), populate the `source_machine_id` field.

- [ ] **Step 5: Run tests**

Run: `make test/backend`

- [ ] **Step 6: Commit**

```bash
git add internal/server/machine_connect.go internal/server/image_connect.go
git commit -m "Add CreateImageFromMachine API handler with exclusion control"
```

---

## Task 9: Frontend — Create Image button and imaging state

**Files:**
- Modify: `web/src/lib/api.ts`
- Modify: `web/src/lib/types.ts`
- Create: `web/src/components/CreateImageDialog.tsx`
- Modify: `web/src/pages/MachineDetailPage.tsx`

- [ ] **Step 1: Add locked_operation to types**

In `web/src/lib/types.ts`, add `lockedOperation` to the Machine type (should be auto-generated from proto, but verify).

- [ ] **Step 2: Add createImageFromMachine API function**

In `web/src/lib/api.ts`:

```typescript
export async function createImageFromMachine(machineId: string, name: string, description: string): Promise<string> {
  const response = await machineClient.createImageFromMachine(
    create(CreateImageFromMachineRequestSchema, { machineId, name, description })
  )
  return response.jobId
}
```

Add the necessary imports for the new schema.

- [ ] **Step 3: Create CreateImageDialog component**

Create `web/src/components/CreateImageDialog.tsx`:
- Dialog with image name input (default: `{machineName}-image-{YYYYMMDD-HHmmss}`, sanitized).
- Optional description textarea.
- Validates image name format (lowercase, hyphens, numbers).
- Submit calls `createImageFromMachine`.
- On success, shows toast and triggers machine refresh.

- [ ] **Step 4: Add Create Image button to MachineDetailPage**

In `web/src/pages/MachineDetailPage.tsx`:
- Add "Create Image" button in the actions area (near start/stop/delete buttons).
- Visible when `machine.userRole === 'admin' || machine.userRole === 'owner'`.
- Enabled only when `machine.status === 'running' && machine.desiredStatus === 'running' && !machine.lockedOperation`.
- Opens `CreateImageDialog` on click.

- [ ] **Step 5: Show imaging state**

In `MachineDetailPage.tsx`:
- When `machine.lockedOperation === 'create_image'`, show "Creating Image..." overlay/badge alongside the real status.
- Disable start/stop/delete buttons when `lockedOperation` is set.
- Increase polling frequency during imaging (e.g., 5000ms instead of 60000ms).

- [ ] **Step 6: Build and verify**

Run: `make build-frontend`

- [ ] **Step 7: Commit**

```bash
git add web/src/lib/api.ts web/src/lib/types.ts web/src/components/CreateImageDialog.tsx web/src/pages/MachineDetailPage.tsx
git commit -m "Add Create Image UI to machine detail page"
```

---

## Task 10: Custom Images List — source machine link

**Files:**
- Modify: custom images list page (find exact path)

- [ ] **Step 1: Show source machine link**

In the custom images admin page, for images with `sourceMachineId`:
- Show a "Source Machine" column or field with a link to the machine detail page.
- If the machine doesn't exist (deleted), show "Source machine deleted" in muted text.

- [ ] **Step 2: Build and verify**

Run: `make build-frontend`

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/CustomImagesPage.tsx  # or actual file path
git commit -m "Show source machine link on custom images list"
```

---

## Task 11: E2E Tests

**Files:**
- Create: `web/e2e/create-image-from-machine.spec.ts`

- [ ] **Step 1: Write E2E test**

Test the full flow:
1. Navigate to machine detail page for a running machine.
2. Click "Create Image" button.
3. Fill in image name, submit.
4. Verify machine shows imaging state.
5. Wait for imaging to complete (machine returns to running).
6. Verify custom image appears in images list.

Note: This is a slow test (requires actual LXD). Add to the slow project.

- [ ] **Step 2: Run E2E test**

Run: `cd web && npx playwright test --project=fast e2e/create-image-from-machine.spec.ts`

If it involves LXD provisioning:
Run: `cd web && npx playwright test --project=slow e2e/create-image-from-machine.spec.ts`

- [ ] **Step 3: Commit**

```bash
git add web/e2e/create-image-from-machine.spec.ts
git commit -m "Add E2E test for creating custom image from machine"
```

---

## Task 12: Integration Testing and Final Verification

- [ ] **Step 1: Run full backend tests**

Run: `make test/backend`

- [ ] **Step 2: Run E2E tests**

Run: `make test/e2e`

- [ ] **Step 3: Manual verification with LXD**

Using the API token method:
1. Start server: `ARCA_API_TOKEN="dev-token-12345" make run`
2. Create a machine, wait for it to be running.
3. Call `CreateImageFromMachine` via curl.
4. Monitor machine events.
5. Verify custom image appears in list.
6. Create a new machine from the custom image.
7. Verify the new machine boots successfully.

- [ ] **Step 4: Create PR**

```bash
git push -u origin ryotarai/custom-image-from-machine
gh pr create --title "Add custom image creation from machines" --body "..."
```

---

## Task Dependencies

```
Task 1 (migration) ──┬──→ Task 2 (store) ──→ Task 7 (worker job) ──→ Task 12 (final)
                      │                                ↑
                      ├──→ Task 3 (image store) ───────┤
                      │                                ↑
                      ├──→ Task 4 (proto) ──→ Task 8 (API handler) ──→ Task 9 (frontend) ──→ Task 11 (E2E)
                      │                                ↑
                      └──→ Task 5 (runtime) ───────────┤
                                                       ↑
                           Task 6 (arcad) ─────────────┘
                                                       ↑
                                              Task 10 (images list)
```

**Parallelizable groups after Task 1:**
- Group A: Task 2, Task 3 (DB store changes)
- Group B: Task 4 (proto)
- Group C: Task 5 (runtime)
- Group D: Task 6 (arcad)

Tasks 7-8 depend on all of A-D. Task 9 depends on 8. Tasks 10-11 depend on 9.
