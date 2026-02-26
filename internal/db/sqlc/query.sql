-- name: GetMeta :one
SELECT value
FROM app_meta
WHERE key = sqlc.arg(key)
LIMIT 1;

-- name: UpsertMeta :exec
INSERT INTO app_meta (key, value)
VALUES (sqlc.arg(key), sqlc.arg(value))
ON CONFLICT (key) DO UPDATE
SET value = excluded.value;

-- name: CreateUser :exec
INSERT INTO users (id, email, password_hash)
VALUES (sqlc.arg(id), sqlc.arg(email), sqlc.arg(password_hash));

-- name: GetUserByEmail :one
SELECT id, email, password_hash, created_at
FROM users
WHERE email = sqlc.arg(email)
LIMIT 1;

-- name: GetUserByID :one
SELECT id, email, password_hash, created_at
FROM users
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: CreateSession :exec
INSERT INTO sessions (id, user_id, token_hash, expires_at)
VALUES (sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(token_hash), sqlc.arg(expires_at_unix));

-- name: GetUserByActiveSessionTokenHash :one
SELECT u.id, u.email, u.password_hash, u.created_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = sqlc.arg(token_hash)
  AND s.revoked_at IS NULL
  AND s.expires_at > sqlc.arg(now_unix)
LIMIT 1;

-- name: RevokeSessionByTokenHash :exec
UPDATE sessions
SET revoked_at = CURRENT_TIMESTAMP
WHERE token_hash = sqlc.arg(token_hash)
  AND revoked_at IS NULL;

-- name: CreateMachine :exec
INSERT INTO machines (id, name)
VALUES (sqlc.arg(id), sqlc.arg(name));

-- name: CreateUserMachine :exec
INSERT INTO user_machines (user_id, machine_id, role)
VALUES (sqlc.arg(user_id), sqlc.arg(machine_id), sqlc.arg(role));

-- name: UpdateMachineNameByIDForOwner :execrows
UPDATE machines
SET name = sqlc.arg(name)
WHERE id = sqlc.arg(machine_id)
  AND EXISTS (
    SELECT 1
    FROM user_machines um
    WHERE um.machine_id = machines.id
      AND um.user_id = sqlc.arg(user_id)
      AND um.role = 'owner'
  );

-- name: DeleteUserMachineByMachineIDForOwner :execrows
DELETE FROM user_machines
WHERE machine_id = sqlc.arg(machine_id)
  AND user_id = sqlc.arg(user_id)
  AND role = 'owner';

-- name: DeleteMachineIfNoUsers :exec
DELETE FROM machines
WHERE id = sqlc.arg(machine_id)
  AND NOT EXISTS (
    SELECT 1
    FROM user_machines um
    WHERE um.machine_id = machines.id
  );

-- name: CreateMachineState :exec
INSERT INTO machine_states (machine_id, status, desired_status, updated_at)
VALUES (sqlc.arg(machine_id), sqlc.arg(status), sqlc.arg(desired_status), sqlc.arg(updated_at));

-- name: ListMachinesByUser :many
SELECT m.id, m.name, ms.status, ms.desired_status, ms.container_id, ms.last_error
FROM machines m
JOIN user_machines um ON um.machine_id = m.id
JOIN machine_states ms ON ms.machine_id = m.id
WHERE um.user_id = sqlc.arg(user_id)
ORDER BY m.created_at DESC;

-- name: GetMachineByID :one
SELECT m.id, m.name, ms.status, ms.desired_status, ms.container_id, ms.last_error
FROM machines m
JOIN machine_states ms ON ms.machine_id = m.id
WHERE m.id = sqlc.arg(machine_id)
LIMIT 1;

-- name: UpdateMachineStateForOwner :execrows
UPDATE machine_states
SET status = sqlc.arg(status),
    desired_status = sqlc.arg(desired_status),
    updated_at = sqlc.arg(updated_at),
    last_error = ''
WHERE machine_states.machine_id = sqlc.arg(machine_id)
  AND EXISTS (
    SELECT 1
    FROM user_machines um
    WHERE um.machine_id = machine_states.machine_id
      AND um.user_id = sqlc.arg(user_id)
      AND um.role = 'owner'
  );

-- name: UpdateMachineRuntimeStateByMachineID :exec
UPDATE machine_states
SET status = sqlc.arg(status),
    desired_status = sqlc.arg(desired_status),
    container_id = sqlc.arg(container_id),
    last_error = sqlc.arg(last_error),
    updated_at = sqlc.arg(updated_at)
WHERE machine_id = sqlc.arg(machine_id);

-- name: EnqueueMachineJob :exec
INSERT INTO machine_jobs (
  id, machine_id, kind, status, attempt, next_run_at, created_at, updated_at
)
VALUES (
  sqlc.arg(id),
  sqlc.arg(machine_id),
  sqlc.arg(kind),
  'queued',
  0,
  sqlc.arg(next_run_at),
  sqlc.arg(now_unix),
  sqlc.arg(now_unix)
);

-- name: ListRunnableMachineJobs :many
SELECT id, machine_id, kind, attempt
FROM machine_jobs
WHERE status = 'queued'
  AND next_run_at <= sqlc.arg(now_unix)
ORDER BY created_at ASC
LIMIT sqlc.arg(limit_n);

-- name: ClaimMachineJob :execrows
UPDATE machine_jobs
SET status = 'running',
    lease_owner = sqlc.arg(lease_owner),
    lease_until = sqlc.arg(lease_until),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id)
  AND status = 'queued';

-- name: MarkMachineJobSucceeded :exec
UPDATE machine_jobs
SET status = 'succeeded',
    lease_owner = NULL,
    lease_until = NULL,
    last_error = NULL,
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: RequeueMachineJob :exec
UPDATE machine_jobs
SET status = 'queued',
    attempt = attempt + 1,
    next_run_at = sqlc.arg(next_run_at),
    lease_owner = NULL,
    lease_until = NULL,
    last_error = sqlc.arg(last_error),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: RecoverExpiredMachineJobs :execrows
UPDATE machine_jobs
SET status = 'queued',
    lease_owner = NULL,
    lease_until = NULL,
    updated_at = sqlc.arg(updated_at)
WHERE status = 'running'
  AND lease_until IS NOT NULL
  AND lease_until < sqlc.arg(now_unix);
