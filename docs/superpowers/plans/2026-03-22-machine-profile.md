# Machine Profile Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename Machine Template to Machine Profile with dynamic setting propagation to existing machines.

**Architecture:** Split the current snapshot-only model into a hybrid: machines keep a live FK reference to their profile (for dynamic settings like auto_stop, server_api_url, startup scripts) while freezing infrastructure settings (network, storage, provider connection) at creation time. A `boot_config_hash` mechanism detects when running machines need a restart.

**Tech Stack:** Go 1.22, SQLite/PostgreSQL (golang-migrate), ConnectRPC (buf/protobuf), React + shadcn/ui + Tailwind CSS, sqlc

**Spec:** `docs/superpowers/specs/2026-03-22-machine-profile-design.md`

---

## File Structure

### Files to Create
- `internal/db/migrations_v2/000045_rename_template_to_profile.up.sql` — DB migration
- `proto/arca/v1/machine_profile.proto` — New proto replacing machine_template.proto
- `internal/server/machine_profile_connect.go` — New handler replacing machine_template_connect.go
- `internal/db/machine_profile_store.go` — New store replacing machine_template_store.go
- `web/src/pages/MachineProfileFormPage.tsx` — New page replacing MachineTemplateFormPage.tsx
- `web/src/pages/MachineProfileDetailPage.tsx` — New page replacing MachineTemplateDetailPage.tsx
- `web/src/pages/MachineProfilesListPage.tsx` — New page replacing MachineTemplatesListPage.tsx

### Files to Modify (Major)
- `internal/db/sqlc/schema.sql` — Rename tables/columns, add new columns
- `internal/db/sqlc/query.sql` — Rename queries, add profile-aware queries
- `internal/db/machine_store.go` — Rename template fields to profile, add profile-aware methods
- `internal/db/machine_exposure_method.go` — Rename functions from Template to Profile
- `internal/server/machine_connect.go` — Use profile live reference for dynamic settings
- `internal/machine/worker.go` — Read dynamic settings from profile instead of snapshot
- `internal/machine/routing_template.go` — Read infrastructure from frozen config, dynamic from profile
- `internal/machine/cloud_init.go` — Startup script from profile (live) instead of snapshot
- `proto/arca/v1/machine.proto` — Rename template fields, add profile fields, add restart_needed
- `web/src/lib/api.ts` — Rename template functions to profile
- `web/src/App.tsx` — Update routes
- `web/src/pages/CreateMachinePage.tsx` — Template → Profile terminology
- `web/src/pages/MachineDetailPage.tsx` — Add source labels, restart-needed banner
- `web/src/pages/MachinesPage.tsx` — Add sync status column
- `web/src/pages/MachineEditPage.tsx` — Profile change when stopped

### Files to Delete (after new files are in place)
- `proto/arca/v1/machine_template.proto`
- `internal/server/machine_template_connect.go`
- `internal/db/machine_template_store.go`
- `web/src/pages/MachineTemplateFormPage.tsx`
- `web/src/pages/MachineTemplateDetailPage.tsx`
- `web/src/pages/MachineTemplatesListPage.tsx`

---

## Task 1: Database Migration — Rename Tables and Columns

**Files:**
- Create: `internal/db/migrations_v2/000045_rename_template_to_profile.up.sql`
- Modify: `internal/db/sqlc/schema.sql`

- [ ] **Step 1: Write the migration SQL**

Create `internal/db/migrations_v2/000045_rename_template_to_profile.up.sql`:

```sql
-- Rename machine_templates -> machine_profiles
ALTER TABLE machine_templates RENAME TO machine_profiles;

-- Add boot_config_hash to machine_profiles
ALTER TABLE machine_profiles ADD COLUMN boot_config_hash TEXT NOT NULL DEFAULT '';

-- Backfill boot_config_hash for existing profiles from their config_json.
-- The boot_config_hash covers startup_script fields from provider configs.
-- At migration time, set a placeholder value that forces all existing machines
-- to show restart_needed=true once, which is the safe default.
UPDATE machine_profiles SET boot_config_hash = 'migrated-' || id;

-- Rename template_custom_images -> profile_custom_images
ALTER TABLE template_custom_images RENAME TO profile_custom_images;

-- Rename columns in custom_images
ALTER TABLE custom_images RENAME COLUMN template_type TO provider_type;

-- Rename columns in machines and add FK constraint.
-- SQLite requires create-copy-drop-rename for adding FK constraints.
-- Check existing migration patterns in this project and follow the same approach.
-- The new table must have:
--   profile_id (renamed from template_id) with REFERENCES machine_profiles(id) ON DELETE RESTRICT
--   provider_type (renamed from template_type)
--   infrastructure_config_json (renamed from template_config_json)
--   applied_boot_config_hash (new, TEXT NOT NULL DEFAULT '')
-- All other columns remain unchanged.
ALTER TABLE machines RENAME COLUMN template_id TO profile_id;
ALTER TABLE machines RENAME COLUMN template_type TO provider_type;
ALTER TABLE machines RENAME COLUMN template_config_json TO infrastructure_config_json;
ALTER TABLE machines ADD COLUMN applied_boot_config_hash TEXT NOT NULL DEFAULT '';

-- Strip dynamic settings from existing machines' infrastructure_config_json.
-- Dynamic settings (server_api_url, auto_stop_timeout_seconds, startup scripts)
-- should be removed from the frozen config since they now come from the live profile.
-- Use json_remove to strip known dynamic keys. Provider-specific startup_script
-- fields are nested inside the provider object (e.g., $.libvirt.startup_script).
UPDATE machines SET infrastructure_config_json = json_remove(
    json_remove(
        json_remove(
            json_remove(
                json_remove(infrastructure_config_json,
                    '$.serverApiUrl'),
                '$.server_api_url'),
            '$.autoStopTimeoutSeconds'),
        '$.auto_stop_timeout_seconds'),
    '$.libvirt.startup_script',
    '$.libvirt.startupScript',
    '$.gce.startup_script',
    '$.gce.startupScript',
    '$.lxd.startup_script',
    '$.lxd.startupScript'
)
WHERE infrastructure_config_json != '' AND infrastructure_config_json != '{}';
```

**Notes:**
- Check existing migrations for the SQLite column rename pattern used in this project. If `ALTER TABLE RENAME COLUMN` is used elsewhere, follow that pattern. If not, use the create-new-table-copy-drop-rename approach.
- The FK constraint (`ON DELETE RESTRICT`) on `profile_id` requires the create-copy-drop-rename approach in SQLite. Add it in this migration.
- The `boot_config_hash` backfill uses a unique placeholder per profile to force a one-time restart_needed=true after migration. The correct hash will be computed when the profile is next saved or on first server startup via a backfill step in the application code.
- The `json_remove` for stripping dynamic settings covers both camelCase and snake_case key variants since the JSON encoding may vary.

- [ ] **Step 2: Update schema.sql to reflect new names**

In `internal/db/sqlc/schema.sql`:
- Rename `machine_templates` table → `machine_profiles`, add `boot_config_hash TEXT NOT NULL DEFAULT ''`
- Rename `template_custom_images` → `profile_custom_images`, rename `template_id` → `profile_id`
- In `custom_images`: rename `template_type` → `provider_type`
- In `machines`: rename `template_id` → `profile_id`, `template_type` → `provider_type`, `template_config_json` → `infrastructure_config_json`, add `applied_boot_config_hash TEXT NOT NULL DEFAULT ''`

- [ ] **Step 3: Verify migration applies cleanly**

```bash
# Build and run server to test migration
cd <worktree> && make build-server
```

- [ ] **Step 4: Commit**

```bash
git add internal/db/migrations_v2/000045_rename_template_to_profile.up.sql internal/db/sqlc/schema.sql
git commit -m "Add migration to rename machine_templates to machine_profiles"
```

---

## Task 2: Update SQL Queries and Regenerate sqlc

**Files:**
- Modify: `internal/db/sqlc/query.sql`
- Regenerate: `internal/db/sqlc/sqlite/*.go`, `internal/db/sqlc/postgresql/*.go`

- [ ] **Step 1: Rename all template queries to profile in query.sql**

Rename queries and update table/column references:
- `ListMachineTemplates` → `ListMachineProfiles` (use `machine_profiles` table)
- `CreateMachineTemplate` → `CreateMachineProfile` (include `boot_config_hash`)
- `GetMachineTemplateByID` → `GetMachineProfileByID`
- `UpdateMachineTemplateByID` → `UpdateMachineProfileByID` (include `boot_config_hash`)
- `DeleteMachineTemplateByID` → `DeleteMachineProfileByID`
- `CreateMachine` → update column names (`profile_id`, `provider_type`, `infrastructure_config_json`, `applied_boot_config_hash`)
- All queries referencing `template_id` → `profile_id`
- All queries referencing `template_type` → `provider_type`
- All queries referencing `template_config_json` → `infrastructure_config_json`
- `template_custom_images` → `profile_custom_images`
- `custom_images.template_type` → `custom_images.provider_type`

- [ ] **Step 2: Add new queries for profile-aware machine operations**

Add to `query.sql`:

```sql
-- name: GetMachineProfileByIDForBootHash :one
SELECT boot_config_hash FROM machine_profiles WHERE id = ?;

-- name: CountMachinesByProfileID :one
SELECT COUNT(*) FROM machines WHERE profile_id = ?;

-- name: CountMachinesByProfileIDAndMachineType :one
SELECT COUNT(*) FROM machines
WHERE profile_id = ?
AND json_extract(options_json, '$.machine_type') = ?;

-- name: UpdateMachineProfileID :exec
UPDATE machines SET profile_id = ? WHERE id = ?;

-- name: UpdateMachineAppliedBootConfigHash :exec
UPDATE machines SET applied_boot_config_hash = ? WHERE id = ?;
```

- [ ] **Step 3: Regenerate sqlc code**

```bash
cd <worktree> && make sqlc
```

- [ ] **Step 4: Verify generated code compiles**

```bash
cd <worktree> && go vet ./internal/db/sqlc/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/db/sqlc/
git commit -m "Update SQL queries for machine profile rename and regenerate sqlc"
```

---

## Task 3: Profile Store — Rename and Add boot_config_hash

**Files:**
- Create: `internal/db/machine_profile_store.go`

- [ ] **Step 1: Write tests for profile store**

Create `internal/db/machine_profile_store_test.go` with table-driven tests:
- Test `CreateMachineProfile` stores boot_config_hash
- Test `UpdateMachineProfileByID` updates boot_config_hash
- Test `DeleteMachineProfileByID` returns error when machines reference profile
- Test `GetMachineProfileByID` returns correct struct

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd <worktree> && go test ./internal/db/ -run TestMachineProfile -v
```

- [ ] **Step 3: Create machine_profile_store.go**

Copy `machine_template_store.go` → `machine_profile_store.go` and rename:
- Struct `MachineTemplate` → `MachineProfile`, add `BootConfigHash string` field
- Constants: keep `TemplateTypeLibvirt` etc. as-is (they represent provider types, aliased later)
- All methods: `*MachineTemplate*` → `*MachineProfile*`
- `countMachinesByTemplateID` → `countMachinesByProfileID`
- Update SQL query names to match renamed queries from Task 2
- Add `BootConfigHash` field to all scan operations
- `CreateMachineProfile` and `UpdateMachineProfileByID` must compute `boot_config_hash` from boot-time settings in config_json before storing

- [ ] **Step 4: Add boot_config_hash computation**

Add function `computeBootConfigHash(configJSON string) string` that:
1. Parses config_json
2. Extracts boot-time fields: startup_script (from provider config), cloud-init related fields
3. Serializes extracted fields canonically (sorted JSON keys)
4. Returns SHA-256 hex hash

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd <worktree> && go test ./internal/db/ -run TestMachineProfile -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/db/machine_profile_store.go internal/db/machine_profile_store_test.go
git commit -m "Add machine profile store with boot_config_hash computation"
```

**Note:** Do NOT delete `machine_template_store.go` yet — it is still referenced by other code. It will be deleted in Task 4 after all callers are updated.

---

## Task 4: Machine Store — Rename Template Fields to Profile

**Files:**
- Modify: `internal/db/machine_store.go`
- Modify: `internal/db/machine_exposure_method.go`

- [ ] **Step 1: Rename struct fields in machine_store.go**

In `Machine` struct:
- `TemplateID` → `ProfileID`
- `TemplateType` → `ProviderType`
- `TemplateConfigJSON` → `InfrastructureConfigJSON`

In `CreateMachineOptions`:
- `TemplateType` → `ProviderType`
- `TemplateConfigJSON` → `InfrastructureConfigJSON`

Add field: `AppliedBootConfigHash string` to `Machine` struct.

Update `CreateMachineWithOwner` and `createMachineWithOwnerOpts` to use new column names.

Update all scan operations to use renamed columns.

- [ ] **Step 2: Add UpdateMachineProfileID method**

```go
func (s *Store) UpdateMachineProfileID(ctx context.Context, machineID, profileID string) error {
    _, err := s.db.ExecContext(ctx, "UPDATE machines SET profile_id = ? WHERE id = ?", profileID, machineID)
    return err
}
```

- [ ] **Step 3: Add UpdateMachineAppliedBootConfigHash method**

```go
func (s *Store) UpdateMachineAppliedBootConfigHash(ctx context.Context, machineID, hash string) error {
    _, err := s.db.ExecContext(ctx, "UPDATE machines SET applied_boot_config_hash = ? WHERE id = ?", hash, machineID)
    return err
}
```

- [ ] **Step 4: Rename functions in machine_exposure_method.go**

Rename all `GetTemplate*` functions to `GetProfile*`:
- `GetTemplateExposureMethod` → `GetProfileExposureMethod`
- `GetTemplateExposureConfig` → `GetProfileExposureConfig`
- `GetTemplateConnectivity` → `GetProfileConnectivity`
- `GetTemplateServerAPIURL` → `GetProfileServerAPIURL`
- `GetTemplateAutoStopTimeoutSeconds` → `GetProfileAutoStopTimeoutSeconds`

Rename `TemplateExposureConfig` → `ProfileExposureConfig`.

**Important:** These functions now take `configJSON` which may come from either the profile (live) or the machine's frozen infrastructure config. The caller decides which to pass. The function signatures stay the same (they take a `string` configJSON), only the names change.

- [ ] **Step 5: Fix all compilation errors**

Search for all references to old names across the codebase and update them:
```bash
cd <worktree> && grep -rn 'TemplateID\|TemplateType\|TemplateConfigJSON\|GetTemplate' --include='*.go' | grep -v '_test.go' | grep -v 'sqlc/'
```

Update every reference. Also update custom image store/handler code:
- References to `template_custom_images` → `profile_custom_images`
- References to `template_id` in junction table → `profile_id`
- References to `template_type` in custom_images → `provider_type`

- [ ] **Step 6: Delete machine_template_store.go**

All callers have been updated. Safe to delete now.
```bash
rm internal/db/machine_template_store.go
```

- [ ] **Step 7: Verify compilation**

```bash
cd <worktree> && go vet ./...
```

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "Rename template fields to profile in machine store, helpers, and custom images"
```

---

## Task 5: Protobuf — Rename Template to Profile

**Files:**
- Create: `proto/arca/v1/machine_profile.proto`
- Modify: `proto/arca/v1/machine.proto`
- Delete: `proto/arca/v1/machine_template.proto`

- [ ] **Step 1: Create machine_profile.proto**

Copy `machine_template.proto` → `machine_profile.proto`. Rename:
- `MachineTemplateService` → `MachineProfileService`
- `MachineTemplate` → `MachineProfile`
- `MachineTemplateConfig` → `MachineProfileConfig`
- `MachineTemplateType` → `MachineProfileType` (keep enum values but rename: `MACHINE_TEMPLATE_TYPE_*` → `MACHINE_PROFILE_TYPE_*`)
- `LibvirtTemplateConfig` → `LibvirtProfileConfig`, etc.
- All request/response messages: `*MachineTemplate*` → `*MachineProfile*`
- `MachineTemplateSummary` → `MachineProfileSummary`
- `ListAvailableMachineTemplates` → `ListAvailableProfiles`

Add to `MachineProfile` message:
```proto
string boot_config_hash = 7;
```

- [ ] **Step 2: Update machine.proto**

In `Machine` message:
- `template_id` (field 7) → `profile_id` (keep field number 7 for wire compatibility)
- `template_type` (field 14) → `provider_type`
- `template_config_json` (field 15) → `infrastructure_config_json`
- Add: `bool restart_needed = 17;`

In `CreateMachineRequest`:
- `template_id` (field 2) → `profile_id`

In `MachineService`:
- Add: `rpc UpdateMachineProfile(UpdateMachineProfileRequest) returns (UpdateMachineProfileResponse);`

Add messages:
```proto
message UpdateMachineProfileRequest {
  string machine_id = 1;
  string profile_id = 2;
}
message UpdateMachineProfileResponse {
  Machine machine = 1;
}
```

- [ ] **Step 3: Delete machine_template.proto**

```bash
rm proto/arca/v1/machine_template.proto
```

- [ ] **Step 4: Regenerate proto code**

```bash
cd <worktree> && make proto
```

- [ ] **Step 5: Fix import paths in Go code**

Update all imports from generated template types to profile types. Fix compilation errors.

- [ ] **Step 6: Verify compilation**

```bash
cd <worktree> && go vet ./...
```

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "Rename protobuf definitions from template to profile"
```

---

## Task 6: Profile Connect Handler — Replace Template Handler

**Files:**
- Create: `internal/server/machine_profile_connect.go`
- Modify: `internal/server/server.go` (or wherever handlers are registered)
- Delete: `internal/server/machine_template_connect.go`

- [ ] **Step 1: Create machine_profile_connect.go**

Copy `machine_template_connect.go` → `machine_profile_connect.go`. Rename:
- `machineTemplateConnectService` → `machineProfileConnectService`
- All methods to use `MachineProfile` proto types
- Store calls: `store.CreateMachineTemplate` → `store.CreateMachineProfile`, etc.
- Validation: `validateTemplateRequest` → `validateProfileRequest`
- JSON marshal/unmarshal: `marshalTemplateConfigJSON` → `marshalProfileConfigJSON`

Key logic change in `UpdateMachineProfile`:
- When updating `allowed_machine_types`, check that no existing machine uses a type being removed:
```go
// For each removed machine type, check no machines use it
removedTypes := findRemovedTypes(oldConfig, newConfig)
for _, mt := range removedTypes {
    count, err := s.store.CountMachinesByProfileIDAndMachineType(ctx, profileID, mt)
    if count > 0 {
        return nil, connect.NewError(connect.CodeFailedPrecondition,
            fmt.Errorf("cannot remove machine type %q: %d machines use it", mt, count))
    }
}
```

- [ ] **Step 2: Update save confirmation data in response**

When saving a profile, include machine impact counts in the response. Add to the `UpdateMachineProfile` response:
- Count of machines using this profile (for "Immediate" impact display)
- Count of running machines (for "On next start" impact display)

This may require a new query or extending the existing one.

- [ ] **Step 3: Register new handler, remove old**

In the server setup file (find where `machineTemplateConnectService` is registered), replace with `machineProfileConnectService`.

- [ ] **Step 4: Delete machine_template_connect.go**

```bash
rm internal/server/machine_template_connect.go
```

- [ ] **Step 5: Verify compilation and run backend tests**

```bash
cd <worktree> && make test/backend
```

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "Replace machine template handler with machine profile handler"
```

---

## Task 7: Machine Connect Handler — Live Profile Reference

**Files:**
- Modify: `internal/server/machine_connect.go`

- [ ] **Step 1: Update CreateMachine to snapshot only infrastructure**

In `CreateMachine()`:
1. Fetch profile (was: fetch template)
2. Extract infrastructure-only config from profile's config_json
3. Pass infrastructure config (not full config) as `InfrastructureConfigJSON`
4. Pass profile's `boot_config_hash` as `AppliedBootConfigHash`

Add helper function:
```go
func extractInfrastructureConfig(configJSON string) (string, error) {
    // Parse config_json, keep only: provider connection, network, storage, base image, exposure
    // Remove: startup_script, server_api_url, auto_stop_timeout_seconds
    // Return re-serialized JSON
}
```

- [ ] **Step 2: Update GetMachine to resolve effective config and restart_needed**

In `GetMachine()` and `toMachineMessageWithAdmin()`:
1. Fetch the machine's profile from DB
2. Compute `restart_needed`: compare `machine.AppliedBootConfigHash` with `profile.BootConfigHash`
3. Set `restart_needed` field on proto response
4. For admin config display: merge infrastructure (frozen) + profile (live) config

- [ ] **Step 3: Add UpdateMachineProfile RPC handler**

Implement the new `UpdateMachineProfile` RPC:
```go
func (s *machineConnectService) UpdateMachineProfile(ctx context.Context, req *connect.Request[...]) (*connect.Response[...], error) {
    // 1. Verify caller is admin for this machine
    // 2. Verify machine is stopped
    // 3. Fetch new profile
    // 4. Verify new profile has same provider_type as machine.ProviderType
    // 5. Verify machine's current machine_type is in new profile's allowed_machine_types
    // 6. Update machine's profile_id
    // 7. Return updated machine
}
```

- [ ] **Step 4: Update allowed_machine_types validation in UpdateMachine**

When user changes `machine_type`, fetch the allowed list from the **live profile** (not the frozen config):
```go
profile, err := s.dbStore.GetMachineProfileByID(ctx, machine.ProfileID)
// validate against profile's allowed_machine_types
```

- [ ] **Step 5: Ensure arcad backward compatibility**

Older arcad versions may send requests using old field names. In any endpoints that arcad calls (machine token/auth, status reporting), ensure both old and new field names are accepted. New fields should be additive and optional. This is a CLAUDE.md requirement: "maintain compatibility with older arcad versions."

- [ ] **Step 6: Verify compilation**

```bash
cd <worktree> && go vet ./...
```

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "Update machine handler to use live profile reference"
```

---

## Task 8: Worker — Read Dynamic Settings from Profile

**Files:**
- Modify: `internal/machine/worker.go`
- Modify: `internal/machine/routing_template.go`
- Modify: `internal/machine/cloud_init.go`

- [ ] **Step 1: Update worker to fetch profile for dynamic settings**

In `handleStart()`:
1. After fetching machine, also fetch its profile: `store.GetMachineProfileByID(ctx, machine.ProfileID)`
2. Read `server_api_url` from **profile** (live): `db.GetProfileServerAPIURL(profile.ConfigJSON)`
3. Read `startup_script` from **profile** (live): extract from profile's provider config
4. After successful start, update `applied_boot_config_hash`: `store.UpdateMachineAppliedBootConfigHash(ctx, machine.ID, profile.BootConfigHash)`

In `autoStopMachines()`:
1. For each running machine, fetch its profile
2. Read `auto_stop_timeout_seconds` from **profile** (live): `db.GetProfileAutoStopTimeoutSeconds(profile.ConfigJSON)`

- [ ] **Step 2: Update routing_template.go**

In `runtimeForMachine()`:
- Infrastructure config (provider connection, network, storage): read from `machine.InfrastructureConfigJSON` (frozen)
- Startup script: receive from caller (worker passes it from profile)

Update `RuntimeStartOptions` to include startup script from profile rather than extracting from machine's frozen config.

- [ ] **Step 3: Update cloud_init.go**

No structural changes needed — `cloudInitUserData` already takes `RuntimeStartOptions` which includes `StartupScript`. The change is in the caller (worker) which now passes the script from the profile.

- [ ] **Step 4: Add Store interface method for profile lookup**

The worker uses a `Store` interface. Add `GetMachineProfileByID` to that interface so the worker can fetch profiles.

- [ ] **Step 5: Verify compilation and run backend tests**

```bash
cd <worktree> && make test/backend
```

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "Read dynamic settings from live profile in worker"
```

---

## Task 9: Frontend — Rename Template to Profile (API Layer)

**Files:**
- Modify: `web/src/lib/api.ts`
- Modify: `web/src/App.tsx`

- [ ] **Step 1: Update api.ts**

Rename all template functions and types to profile:
- `listMachineTemplates` → `listMachineProfiles`
- `createMachineTemplate` → `createMachineProfile`
- `updateMachineTemplate` → `updateMachineProfile`
- `deleteMachineTemplate` → `deleteMachineProfile`
- `listAvailableMachineTemplates` → `listAvailableProfiles`
- Import generated profile client instead of template client
- Add `updateMachineProfile(machineId, profileId)` function for profile reassignment

- [ ] **Step 2: Update App.tsx routes**

Replace template routes with profile routes:
- `/machine-templates` → `/machine-profiles`
- `/machine-templates/new` → `/machine-profiles/new`
- `/machine-templates/:templateID` → `/machine-profiles/:profileID`
- `/machine-templates/:templateID/edit` → `/machine-profiles/:profileID/edit`

- [ ] **Step 3: Verify frontend builds**

```bash
cd <worktree> && make build-frontend
```

- [ ] **Step 4: Commit**

```bash
git add web/src/lib/api.ts web/src/App.tsx
git commit -m "Rename template to profile in frontend API layer and routes"
```

---

## Task 10: Frontend — Profile Edit Page with Setting Categories

**Files:**
- Create: `web/src/pages/MachineProfileFormPage.tsx`
- Delete: `web/src/pages/MachineTemplateFormPage.tsx`

- [ ] **Step 1: Create MachineProfileFormPage.tsx**

Based on `MachineTemplateFormPage.tsx`, redesign the form with four sections:

**Section 1: "Applies immediately"** (with explanatory subtitle)
- Auto-stop timeout
- Server API URL

**Section 2: "Applies on next start"** (with explanatory subtitle)
- Startup script (provider-specific)

**Section 3: "New machines only"** (with explanatory subtitle)
- Provider connection fields (URI/project/zone/endpoint)
- Network / subnet
- Storage pool
- Exposure config

**Section 4: "Constraints"**
- Allowed machine types (GCE only)

Provider type: shown as read-only on edit (immutable after creation). Selectable only on create.

- [ ] **Step 2: Add save confirmation dialog**

When user clicks Save on an existing profile, show a dialog:
- Fetch machine count for this profile
- Fetch running machine count
- Group changed fields by category
- Display: "Immediate (N machines): field X changed", "On next start (M running machines): field Y changed", "New machines only: field Z changed"

Use shadcn/ui `AlertDialog` component.

- [ ] **Step 3: Delete MachineTemplateFormPage.tsx**

```bash
rm web/src/pages/MachineTemplateFormPage.tsx
```

- [ ] **Step 4: Verify frontend builds**

```bash
cd <worktree> && make build-frontend
```

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "Add profile edit page with setting categories and save confirmation"
```

---

## Task 11: Frontend — Profile List and Detail Pages

**Files:**
- Create: `web/src/pages/MachineProfilesListPage.tsx`
- Create: `web/src/pages/MachineProfileDetailPage.tsx`
- Delete: `web/src/pages/MachineTemplatesListPage.tsx`
- Delete: `web/src/pages/MachineTemplateDetailPage.tsx`

- [ ] **Step 1: Create MachineProfilesListPage.tsx**

Based on `MachineTemplatesListPage.tsx`, rename all template references to profile.
Show machine count per profile.

- [ ] **Step 2: Create MachineProfileDetailPage.tsx**

Based on `MachineTemplateDetailPage.tsx`, rename all template references.
Add: count of machines using this profile and count needing restart.

- [ ] **Step 3: Delete old template pages**

```bash
rm web/src/pages/MachineTemplatesListPage.tsx web/src/pages/MachineTemplateDetailPage.tsx
```

- [ ] **Step 4: Verify frontend builds**

```bash
cd <worktree> && make build-frontend
```

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "Add profile list and detail pages"
```

---

## Task 12: Frontend — Machine Detail Page (Source Labels + Restart Banner)

**Files:**
- Modify: `web/src/pages/MachineDetailPage.tsx`

- [ ] **Step 1: Add restart-needed banner**

At the top of the machine detail page, if `machine.restart_needed` is true:
```tsx
<Alert variant="warning">
  <AlertTitle>Restart needed</AlertTitle>
  <AlertDescription>
    Profile "{profile.name}" has been updated. Changes will apply on next restart.
  </AlertDescription>
  <Button onClick={handleRestart}>Restart</Button>
</Alert>
```

- [ ] **Step 2: Add source labels to configuration display**

Show each setting with its source:
- Settings from profile: label "Profile"
- Settings from machine options: label "Machine"
- Frozen infrastructure settings: label "Fixed"

Use a subtle badge/tag next to each value (shadcn/ui `Badge` component with `variant="outline"`).

- [ ] **Step 3: Rename all "Template" text to "Profile"**

Update all user-facing strings: "Template" → "Profile".

- [ ] **Step 4: Verify frontend builds**

```bash
cd <worktree> && make build-frontend
```

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "Add source labels and restart-needed banner to machine detail page"
```

---

## Task 13: Frontend — Machine List Page (Sync Column)

**Files:**
- Modify: `web/src/pages/MachinesPage.tsx`

- [ ] **Step 1: Add Sync column to machine list**

Add a column showing sync status:
- If `machine.restart_needed` and status is "running": show "Restart needed" badge (yellow/warning)
- Otherwise: show "Up to date" or no badge (clean state)

- [ ] **Step 2: Rename "Template" column to "Profile"**

Update the column that shows which template a machine belongs to.

- [ ] **Step 3: Verify frontend builds**

```bash
cd <worktree> && make build-frontend
```

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "Add sync status column to machine list page"
```

---

## Task 14: Frontend — Machine Edit Page (Profile Change + Machine Type)

**Files:**
- Modify: `web/src/pages/MachineEditPage.tsx`

- [ ] **Step 1: Add profile change when stopped**

When machine is stopped, show a "Change profile" option:
- Dropdown of profiles with same provider_type
- On change, call `updateMachineProfile(machineId, newProfileId)` API
- Show confirmation: what changes immediately vs on next start

- [ ] **Step 2: Machine type uses live profile constraints**

When changing machine_type on a stopped machine:
- Fetch allowed types from the machine's current **profile** (not frozen config)
- Show dropdown with profile's allowed_machine_types

- [ ] **Step 3: Verify frontend builds**

```bash
cd <worktree> && make build-frontend
```

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "Add profile change and live constraint validation to machine edit page"
```

---

## Task 15: Frontend — Create Machine Page Update

**Files:**
- Modify: `web/src/pages/CreateMachinePage.tsx`

- [ ] **Step 1: Rename template references to profile**

- "Select a template" → "Select a profile"
- `listAvailableMachineTemplates` → `listAvailableProfiles`
- All variable names and UI text

- [ ] **Step 2: Verify frontend builds**

```bash
cd <worktree> && make build-frontend
```

- [ ] **Step 3: Commit**

```bash
git add -A
git commit -m "Rename template to profile in create machine page"
```

---

## Task 16: Full Build + Backend Tests

- [ ] **Step 1: Full build**

```bash
cd <worktree> && make build
```

Fix any remaining compilation errors.

- [ ] **Step 2: Run backend tests**

```bash
cd <worktree> && make test/backend
```

Fix any test failures.

- [ ] **Step 3: Commit fixes if any**

```bash
git add -A
git commit -m "Fix build and test issues from profile rename"
```

---

## Task 17: E2E Tests

**Files:**
- Modify: `web/e2e/template-catalog.spec.ts`
- Modify: `web/e2e/machines.spec.ts`
- Modify: `web/e2e/critical-user-journey.spec.ts`
- Modify: `web/e2e/machine-options.spec.ts`

- [ ] **Step 1: Update template-catalog.spec.ts**

Rename to reference profiles:
- Update URLs from `/machine-templates` to `/machine-profiles`
- Update API calls and assertions
- Update test descriptions

- [ ] **Step 2: Update machine-related specs**

In `machines.spec.ts`, `critical-user-journey.spec.ts`, `machine-options.spec.ts`:
- Update all template references to profile
- Update helper functions (e.g., `ensureLxdTemplate` → `ensureLxdProfile`)

- [ ] **Step 3: Add E2E test for restart-needed indicator**

Add a test that:
1. Creates a profile and machine
2. Updates the profile's startup script
3. Verifies the machine list shows "Restart needed" badge
4. Restarts the machine
5. Verifies badge disappears

- [ ] **Step 4: Run E2E tests**

```bash
cd <worktree>/web && npx playwright test --project=fast
```

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "Update E2E tests for profile rename and add restart-needed test"
```

---

## Task 18: Final Verification

- [ ] **Step 1: Full build**

```bash
cd <worktree> && make build
```

- [ ] **Step 2: Backend tests**

```bash
cd <worktree> && make test/backend
```

- [ ] **Step 3: E2E tests**

```bash
cd <worktree>/web && npx playwright test --project=fast
```

- [ ] **Step 4: Manual smoke test**

Start the server and verify:
1. Profile CRUD works (create, edit with section grouping, delete blocked when in use)
2. Machine creation with profile works
3. Machine detail shows source labels
4. Change profile startup script → machine shows "Restart needed"
5. Change auto_stop_timeout → applies immediately (verify in logs)
6. Change machine_type when stopped
7. Profile change when stopped (same provider type only)

```bash
cd <worktree> && ARCA_API_TOKEN="dev-token-12345" make run
```

- [ ] **Step 5: Commit any remaining fixes**

```bash
git add -A
git commit -m "Final fixes from smoke testing"
```
