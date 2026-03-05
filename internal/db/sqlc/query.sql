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
INSERT INTO machines (id, name, runtime)
VALUES (sqlc.arg(id), sqlc.arg(name), sqlc.arg(runtime));

-- name: UpdateMachineEndpointByID :exec
UPDATE machines
SET endpoint = sqlc.arg(endpoint)
WHERE id = sqlc.arg(machine_id);

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

-- name: DeleteMachineByID :execrows
DELETE FROM machines
WHERE id = sqlc.arg(machine_id);

-- name: CreateMachineState :exec
INSERT INTO machine_states (machine_id, status, desired_status, updated_at)
VALUES (sqlc.arg(machine_id), sqlc.arg(status), sqlc.arg(desired_status), sqlc.arg(updated_at));

-- name: ListMachinesByUser :many
SELECT m.id, m.name, m.runtime, m.endpoint, ms.status, ms.desired_status, ms.container_id, ms.last_error
FROM machines m
JOIN user_machines um ON um.machine_id = m.id
JOIN machine_states ms ON ms.machine_id = m.id
WHERE um.user_id = sqlc.arg(user_id)
ORDER BY m.created_at DESC;

-- name: GetMachineByID :one
SELECT m.id, m.name, m.runtime, m.endpoint, ms.status, ms.desired_status, ms.container_id, ms.last_error
FROM machines m
JOIN machine_states ms ON ms.machine_id = m.id
WHERE m.id = sqlc.arg(machine_id)
LIMIT 1;

-- name: GetMachineByIDForUser :one
SELECT m.id, m.name, m.runtime, m.endpoint, ms.status, ms.desired_status, ms.container_id, ms.last_error
FROM machines m
JOIN machine_states ms ON ms.machine_id = m.id
JOIN user_machines um ON um.machine_id = m.id
WHERE m.id = sqlc.arg(machine_id)
  AND um.user_id = sqlc.arg(user_id)
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

-- name: CreateMachineEvent :exec
INSERT INTO machine_events (id, machine_id, job_id, level, event_type, message, created_at)
VALUES (
  sqlc.arg(id),
  sqlc.arg(machine_id),
  sqlc.arg(job_id),
  sqlc.arg(level),
  sqlc.arg(event_type),
  sqlc.arg(message),
  sqlc.arg(created_at)
);

-- name: ListMachineEventsByMachineIDForUser :many
SELECT me.id, me.machine_id, me.job_id, me.level, me.event_type, me.message, me.created_at
FROM machine_events me
JOIN user_machines um ON um.machine_id = me.machine_id
WHERE me.machine_id = sqlc.arg(machine_id)
  AND um.user_id = sqlc.arg(user_id)
ORDER BY me.created_at DESC
LIMIT sqlc.arg(limit_n);

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

-- name: ListMachinesByDesiredStatus :many
SELECT m.id, m.name, m.runtime, m.endpoint, ms.status, ms.desired_status, ms.container_id, ms.last_error
FROM machines m
JOIN machine_states ms ON ms.machine_id = m.id
WHERE ms.desired_status = sqlc.arg(desired_status)
ORDER BY ms.updated_at ASC
LIMIT sqlc.arg(limit_n);

-- name: CountActiveStartOrReconcileJobsByMachineID :one
SELECT COUNT(1)
FROM machine_jobs
WHERE machine_id = sqlc.arg(machine_id)
  AND status IN ('queued', 'running')
  AND kind IN ('start', 'reconcile');

-- name: GetSetupState :one
SELECT completed, admin_user_id, base_domain, domain_prefix, cloudflare_api_token, docker_provider_enabled, updated_at
FROM setup_state
WHERE id = 1
LIMIT 1;

-- name: UpsertSetupState :exec
INSERT INTO setup_state (id, completed, admin_user_id, base_domain, domain_prefix, cloudflare_api_token, docker_provider_enabled, updated_at)
VALUES (
  1,
  sqlc.arg(completed),
  sqlc.narg(admin_user_id),
  sqlc.arg(base_domain),
  sqlc.arg(domain_prefix),
  sqlc.arg(cloudflare_api_token),
  sqlc.arg(docker_provider_enabled),
  sqlc.arg(updated_at)
)
ON CONFLICT (id) DO UPDATE
SET completed = excluded.completed,
    admin_user_id = excluded.admin_user_id,
    base_domain = excluded.base_domain,
    domain_prefix = excluded.domain_prefix,
    cloudflare_api_token = excluded.cloudflare_api_token,
    docker_provider_enabled = excluded.docker_provider_enabled,
    updated_at = excluded.updated_at;

-- name: CreateMachineToken :exec
INSERT INTO machine_tokens (id, machine_id, token_hash, created_at)
VALUES (sqlc.arg(id), sqlc.arg(machine_id), sqlc.arg(token_hash), sqlc.arg(created_at));

-- name: GetMachineIDByActiveTokenHash :one
SELECT machine_id
FROM machine_tokens
WHERE token_hash = sqlc.arg(token_hash)
  AND revoked_at IS NULL
LIMIT 1;

-- name: CreateAuthTicket :exec
INSERT INTO auth_tickets (id, ticket_hash, user_id, machine_id, exposure_id, expires_at, created_at)
VALUES (
  sqlc.arg(id),
  sqlc.arg(ticket_hash),
  sqlc.arg(user_id),
  sqlc.arg(machine_id),
  sqlc.arg(exposure_id),
  sqlc.arg(expires_at),
  sqlc.arg(created_at)
);

-- name: GetValidAuthTicketByHashAndMachine :one
SELECT t.id, t.user_id, u.email, t.machine_id, t.exposure_id
FROM auth_tickets t
JOIN users u ON u.id = t.user_id
WHERE t.ticket_hash = sqlc.arg(ticket_hash)
  AND t.machine_id = sqlc.arg(machine_id)
  AND t.used_at IS NULL
  AND t.expires_at > sqlc.arg(now_unix)
LIMIT 1;

-- name: MarkAuthTicketUsed :execrows
UPDATE auth_tickets
SET used_at = sqlc.arg(used_at)
WHERE id = sqlc.arg(id)
  AND used_at IS NULL;

-- name: UpsertMachineTunnel :exec
INSERT INTO machine_tunnels (machine_id, account_id, tunnel_id, tunnel_name, tunnel_token, created_at, updated_at)
VALUES (
  sqlc.arg(machine_id),
  sqlc.arg(account_id),
  sqlc.arg(tunnel_id),
  sqlc.arg(tunnel_name),
  sqlc.arg(tunnel_token),
  sqlc.arg(created_at),
  sqlc.arg(updated_at)
)
ON CONFLICT (machine_id) DO UPDATE
SET account_id = excluded.account_id,
    tunnel_id = excluded.tunnel_id,
    tunnel_name = excluded.tunnel_name,
    tunnel_token = excluded.tunnel_token,
    updated_at = excluded.updated_at;

-- name: GetMachineTunnelByMachineID :one
SELECT machine_id, account_id, tunnel_id, tunnel_name, tunnel_token, created_at, updated_at
FROM machine_tunnels
WHERE machine_id = sqlc.arg(machine_id)
LIMIT 1;

-- name: UpsertMachineExposure :exec
INSERT INTO machine_exposures (id, machine_id, name, hostname, service, is_public, created_at, updated_at)
VALUES (
  sqlc.arg(id),
  sqlc.arg(machine_id),
  sqlc.arg(name),
  sqlc.arg(hostname),
  sqlc.arg(service),
  sqlc.arg(is_public),
  sqlc.arg(created_at),
  sqlc.arg(updated_at)
)
ON CONFLICT (machine_id, name) DO UPDATE
SET hostname = excluded.hostname,
    service = excluded.service,
    is_public = excluded.is_public,
    updated_at = excluded.updated_at;

-- name: ListMachineExposuresByMachineID :many
SELECT id, machine_id, name, hostname, service, is_public, created_at, updated_at
FROM machine_exposures
WHERE machine_id = sqlc.arg(machine_id)
ORDER BY created_at ASC;

-- name: GetMachineExposureByHostname :one
SELECT id, machine_id, name, hostname, service, is_public, created_at, updated_at
FROM machine_exposures
WHERE hostname = sqlc.arg(hostname)
LIMIT 1;

-- name: GetMachineExposureByMachineIDAndName :one
SELECT id, machine_id, name, hostname, service, is_public, created_at, updated_at
FROM machine_exposures
WHERE machine_id = sqlc.arg(machine_id)
  AND name = sqlc.arg(name)
LIMIT 1;
