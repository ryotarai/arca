# User Startup Script

## Summary

Allow users to configure a personal startup script in their Settings page. The script runs on every machine they create, after the template's startup script, as the `arcauser` (interactive) user.

## Data Layer

### Migration (`000045_user_startup_script.up.sql`)

```sql
ALTER TABLE users ADD COLUMN startup_script TEXT NOT NULL DEFAULT '';
```

### Schema & Queries

Add `startup_script` column to `users` table in `schema.sql`.

New queries in `query.sql`:

```sql
-- name: GetUserStartupScript :one
SELECT startup_script FROM users WHERE id = sqlc.arg(user_id);

-- name: UpdateUserStartupScript :exec
UPDATE users SET startup_script = sqlc.arg(startup_script) WHERE id = sqlc.arg(user_id);
```

## API Layer

### Proto (`proto/arca/v1/user.proto`)

Add two RPCs to `UserService`:

```protobuf
rpc GetUserStartupScript(GetUserStartupScriptRequest) returns (GetUserStartupScriptResponse);
rpc UpdateUserStartupScript(UpdateUserStartupScriptRequest) returns (UpdateUserStartupScriptResponse);
```

Messages:

```protobuf
message GetUserStartupScriptRequest {}
message GetUserStartupScriptResponse {
  string startup_script = 1;
}

message UpdateUserStartupScriptRequest {
  string startup_script = 1;
}
message UpdateUserStartupScriptResponse {
  string startup_script = 1;
}
```

Authentication: normal user auth (`authenticateUser`). Each user can only read/write their own startup script.

### Handler (`internal/server/user_connect.go`)

- `GetUserStartupScript`: authenticate user, call `store.GetUserStartupScript(ctx, userID)`, return script.
- `UpdateUserStartupScript`: authenticate user, call `store.UpdateUserStartupScript(ctx, userID, script)`, return updated script.

## Machine Startup Flow

### Worker change (`internal/machine/worker.go` — `handleStart`)

After resolving the template's startup script, fetch the machine owner's startup script:

1. Call `store.GetMachineOwnerUserID(ctx, machine.ID)` to get the owner user ID.
2. Call `store.GetUserStartupScript(ctx, ownerUserID)` to get the user's script.
3. Pass both to `RuntimeStartOptions` (new field `UserStartupScript`).

### Cloud-init change (`internal/machine/cloud_init.go`)

Concatenate template script and user script:

```go
startupScript := opts.StartupScript
if userScript := strings.TrimSpace(opts.UserStartupScript); userScript != "" {
    startupScript = startupScript + "\n" + userScript
}
```

The combined script is placed at `/usr/local/bin/arca-user-startup.sh` as before.

### Ansible change (`internal/arcad/ansible/roles/user_startup/tasks/main.yml`)

Change execution to run as `arcauser` instead of root:

```yaml
- name: Run user startup script
  ansible.builtin.shell: "/usr/bin/env bash {{ user_startup_script }}"
  become_user: "{{ interactive_user }}"
  when: startup_script_stat.stat.exists
```

Users can `sudo` for root operations since `arcauser` has `NOPASSWD:ALL`.

## Frontend

### Settings Page (`web/src/pages/SettingsPage.tsx`)

Add a new `StartupScriptCard` component between the header and LLM Models card:

- Monospace `<textarea>` for script content
- Description: "This script runs on every machine you create, after the template startup script. It executes as the arcauser user (sudo available)."
- Save button with loading/success/error states
- Calls `getUserStartupScript()` on mount, `updateUserStartupScript(script)` on save

### API client (`web/src/lib/api.ts`)

Add two functions:

- `getUserStartupScript(): Promise<string>` — calls `GetUserStartupScript`
- `updateUserStartupScript(script: string): Promise<string>` — calls `UpdateUserStartupScript`

## Execution Order

1. cloud-init writes combined script to `/usr/local/bin/arca-user-startup.sh`
2. cloud-init runs `arca-install.sh` (downloads arcad, starts service)
3. arcad runs Ansible setup
4. `user_startup` role executes the script as `arcauser`
5. Remaining Ansible roles continue (systemd_units, sentinel, etc.)

## Non-goals

- No per-machine script override (only user-level)
- No script execution history or logs beyond what arcad already captures
- No size limit enforcement (rely on cloud-init limits)
