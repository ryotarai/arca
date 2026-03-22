# Machine Profile Design

> Rename Machine Template to Machine Profile and enable dynamic propagation of profile settings to existing machines.

## Problem

Machine Templates use a copy-on-create (snapshot) model. When a template is updated, existing machines are unaffected. This is operationally painful — administrators must recreate machines to apply configuration changes like auto-stop timeouts, server API URLs, or startup scripts.

The name "template" reinforces this static, stamp-out-and-detach mental model. A new concept is needed that supports live propagation of settings.

## Solution

Introduce **Machine Profile** — a named configuration that machines reference at runtime. Profile changes propagate to existing machines at a timing appropriate to each setting's nature.

## Data Model

### machine_profiles (renamed from machine_templates)

| Column | Type | Notes |
|--------|------|-------|
| `id` | TEXT PK | |
| `name` | TEXT UNIQUE | |
| `type` | TEXT | Provider type (libvirt/gce/lxd). Immutable after creation. |
| `config_json` | TEXT | Full configuration (infrastructure + policy + constraints) |
| `boot_config_hash` | TEXT | Hash of boot-time settings, updated on save |
| `created_at` | BIGINT | |
| `updated_at` | BIGINT | |

### machines (modified columns)

| Column | Change | Notes |
|--------|--------|-------|
| `profile_id` | Renamed from `template_id` | Live FK reference to `machine_profiles.id`. NOT NULL. |
| `provider_type` | Renamed from `template_type` | Frozen at creation from profile `type`. |
| `infrastructure_config_json` | Renamed from `template_config_json` | Frozen at creation. Contains only infrastructure settings (provider connection, network, storage, base image, exposure). |
| `applied_boot_config_hash` | New | Copied from profile's `boot_config_hash` at machine start. Used to detect restart-needed state. |

Existing columns (`options_json`, `setup_version`, `custom_image_id`, etc.) remain unchanged.

### Configuration Resolution

At runtime, effective configuration is resolved by merging three layers:

```
effective_config = merge(
    machine.infrastructure_config_json,   // frozen infrastructure
    profile.config_json,                  // live policy & boot settings
    machine.options_json                  // per-machine settings
)
```

## Setting Categories

All settings in a profile's `config_json` are classified into four categories by their propagation behavior:

### Infrastructure (new machines only)

Fixed when a machine is created. Changing these on a profile only affects machines created afterward.

- Provider connection (libvirt URI, GCE project/zone, LXD endpoint)
- Network / subnet
- Storage pool
- Base image
- Exposure config

### Boot Settings (next start)

Applied when a machine starts. Running machines pick up changes on their next restart.

- Startup script / cloud-init
- Setup-related configuration

### Runtime Policy (immediate)

Applied on the next arcad check-in. No restart required.

- `auto_stop_timeout_seconds`
- `server_api_url`

### Constraints (validated on mutation)

Enforced when a relevant operation is attempted, not pushed to machines.

- `allowed_machine_types` — validated when machine type is changed or a machine is started.

## Setting Mutability Matrix

### Profile Settings

| Setting | Create | Update | Notes |
|---------|:------:|:------:|-------|
| Provider type | ✅ | ❌ | Immutable after creation. Create a new profile for a different provider. |
| Infrastructure settings | ✅ | ✅ | Affects new machines only. |
| Boot settings | ✅ | ✅ | Affects existing machines on next start. |
| Runtime policy | ✅ | ✅ | Affects existing machines immediately. |
| Allowed machine types | ✅ | ✅ | Cannot remove a type that is in use by any machine. |

### Machine Settings

| Setting | Create | Running | Stopped |
|---------|:------:|:-------:|:-------:|
| Profile | ✅ required | ❌ | ✅ (same provider_type only) |
| Name | ✅ required | ✅ | ✅ |
| Machine type | ✅ optional | ❌ | ✅ (from profile's allowed list) |
| Custom image | ✅ optional | ❌ | ❌ |
| Tags | ✅ optional | ✅ | ✅ |

## Restart-Needed Detection

1. On profile save: compute a hash of boot-time settings → store as `boot_config_hash`.
2. On machine start: copy `profile.boot_config_hash` → `machine.applied_boot_config_hash`.
3. Detection: if `profile.boot_config_hash != machine.applied_boot_config_hash` AND machine is Running → **restart needed**.

This avoids false positives: changes to runtime-policy or infrastructure settings do not trigger the restart badge.

## Profile Deletion

A profile cannot be deleted while machines reference it. The UI prompts the administrator to move machines to another profile first.

## Profile Reassignment

A machine's profile can be changed while the machine is **stopped**:

- The new profile must have the same `provider_type` as the machine's frozen `provider_type`.
- The machine's current `machine_type` must be in the new profile's `allowed_machine_types`.
- The machine's `infrastructure_config_json` is **not** updated (frozen at original creation).
- On next start, boot settings come from the new profile.

## UI Design

### Profile Edit Page

Settings are grouped by propagation timing, with section headers that explain impact:

```
── Applies immediately ──────────────────
  Changes take effect on all machines now.

  Auto-stop timeout     [60 min]
  Server API URL        [http://...]

── Applies on next start ────────────────
  Running machines pick this up on restart.

  Startup script        [Edit...]

── New machines only ────────────────────
  Fixed when a machine is created.
  Existing machines are not affected.

  Network               [default]
  Storage pool          [local]

── Constraints ──────────────────────────

  Allowed machine types [☑ e2-standard-2]
                        [☑ e2-standard-4]
```

**Save confirmation dialog** shows concrete impact:

```
Save profile changes?

Immediate (5 machines):
  Auto-stop timeout: 30 min → 60 min

On next start (3 running machines):
  Startup script: changed

New machines only:
  (no changes)

             [Cancel]  [Save]
```

### Machine Detail Page

Displays effective configuration with source labels and a restart-needed banner:

- Each setting shows its source: **Profile**, **Machine**, or **Fixed** (frozen at creation).
- If restart is needed, a banner appears: "Profile updated. Startup script changed — will apply on next restart." with a **Restart** action.
- Machine type change is available when stopped, showing options from the profile's allowed list.

### Machine List Page

A **Sync** column shows each machine's state relative to its profile:

- **Up to date** — no pending changes (or machine is stopped).
- **Restart needed** — profile has boot-setting changes not yet applied.

### Profile Detail / List Page

Shows the count of machines using each profile and how many need restarts.

## Migration

### Database

1. Rename `machine_templates` → `machine_profiles`.
2. Add `boot_config_hash` column to `machine_profiles` (compute from existing `config_json`).
3. Rename machine columns: `template_id` → `profile_id`, `template_type` → `provider_type`, `template_config_json` → `infrastructure_config_json`.
4. Add `applied_boot_config_hash` column to `machines`.
5. Add FK constraint: `machines.profile_id` → `machine_profiles.id` with `ON DELETE RESTRICT` (enforces deletion rule at DB level).
6. Rename `template_custom_images` → `profile_custom_images` and update its FK. Rename `custom_images.template_type` → `provider_type` for consistency.
7. Strip dynamic settings (runtime policy, boot settings) from each machine's `infrastructure_config_json`, keeping only infrastructure fields.

### API

1. Rename `MachineTemplateService` → `MachineProfileService` with new RPC names.
2. Update `MachineService` RPCs to use profile terminology.
3. Maintain backward compatibility with older arcad versions: accept both old and new field names during a transition period.

### UI

1. Rename all "Template" references to "Profile" across the frontend.
2. Implement the section-grouped profile edit page.
3. Add source labels and restart-needed indicators to machine views.
4. Add save confirmation dialog with impact summary.
