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

-- name: ListUsers :many
SELECT id, email, password_hash, password_setup_required, role, created_at
FROM users
ORDER BY created_at DESC;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, password_setup_required, role, created_at
FROM users
WHERE email = sqlc.arg(email)
LIMIT 1;

-- name: SearchUsersByEmail :many
SELECT id, email
FROM users
WHERE LOWER(email) LIKE '%' || LOWER(sqlc.arg(query)) || '%'
ORDER BY email ASC
LIMIT sqlc.arg(limit_count);

-- name: GetUserByID :one
SELECT id, email, password_hash, password_setup_required, role, created_at
FROM users
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: GetUserSettingsByUserID :one
SELECT u.id AS user_id,
       COALESCE(s.ssh_public_keys_json, '[]') AS ssh_public_keys_json
FROM users u
LEFT JOIN user_settings s ON s.user_id = u.id
WHERE u.id = sqlc.arg(user_id)
LIMIT 1;

-- name: UpsertUserSettingsByUserID :exec
INSERT INTO user_settings (user_id, ssh_public_keys_json, created_at, updated_at)
VALUES (
  sqlc.arg(user_id),
  sqlc.arg(ssh_public_keys_json),
  sqlc.arg(created_at),
  sqlc.arg(updated_at)
)
ON CONFLICT (user_id) DO UPDATE
SET ssh_public_keys_json = excluded.ssh_public_keys_json,
    updated_at = excluded.updated_at;

-- name: UpdateUserRoleByID :execrows
UPDATE users
SET role = sqlc.arg(role)
WHERE id = sqlc.arg(id);

-- name: UpdateUserPasswordHashByID :execrows
UPDATE users
SET password_hash = sqlc.arg(password_hash)
WHERE id = sqlc.arg(id);

-- name: UpdateUserPasswordSetupRequiredByID :execrows
UPDATE users
SET password_setup_required = sqlc.arg(password_setup_required)
WHERE id = sqlc.arg(id);

-- name: CreateUserSetupToken :exec
INSERT INTO user_setup_tokens (id, token_hash, user_id, created_by_user_id, expires_at, created_at)
VALUES (
  sqlc.arg(id),
  sqlc.arg(token_hash),
  sqlc.arg(user_id),
  sqlc.narg(created_by_user_id),
  sqlc.arg(expires_at),
  sqlc.arg(created_at)
);

-- name: InvalidateUserSetupTokensByUserID :exec
UPDATE user_setup_tokens
SET used_at = sqlc.arg(used_at)
WHERE user_id = sqlc.arg(user_id)
  AND used_at IS NULL;

-- name: GetActiveUserSetupTokenByUserID :one
SELECT id, token_hash, user_id, created_by_user_id, expires_at, used_at, created_at
FROM user_setup_tokens
WHERE user_id = sqlc.arg(user_id)
  AND used_at IS NULL
  AND expires_at > sqlc.arg(now_unix)
ORDER BY created_at DESC
LIMIT 1;

-- name: GetValidUserSetupTokenByHash :one
SELECT t.id, t.token_hash, t.user_id, t.created_by_user_id, t.expires_at, t.used_at, t.created_at, u.email
FROM user_setup_tokens t
JOIN users u ON u.id = t.user_id
WHERE t.token_hash = sqlc.arg(token_hash)
  AND t.used_at IS NULL
  AND t.expires_at > sqlc.arg(now_unix)
LIMIT 1;

-- name: MarkUserSetupTokenUsed :execrows
UPDATE user_setup_tokens
SET used_at = sqlc.arg(used_at)
WHERE id = sqlc.arg(id)
  AND used_at IS NULL;

-- name: CreateArcadExchangeToken :exec
INSERT INTO arcad_exchange_tokens (id, token_hash, user_id, machine_id, exposure_id, expires_at, created_at)
VALUES (
  sqlc.arg(id),
  sqlc.arg(token_hash),
  sqlc.arg(user_id),
  sqlc.arg(machine_id),
  sqlc.arg(exposure_id),
  sqlc.arg(expires_at),
  sqlc.arg(created_at)
);

-- name: GetValidArcadExchangeTokenByHashAndMachine :one
SELECT t.id, t.user_id, u.email, t.machine_id, t.exposure_id
FROM arcad_exchange_tokens t
JOIN users u ON u.id = t.user_id
WHERE t.token_hash = sqlc.arg(token_hash)
  AND t.machine_id = sqlc.arg(machine_id)
  AND t.used_at IS NULL
  AND t.expires_at > sqlc.arg(now_unix)
LIMIT 1;

-- name: MarkArcadExchangeTokenUsed :execrows
UPDATE arcad_exchange_tokens
SET used_at = sqlc.arg(used_at)
WHERE id = sqlc.arg(id)
  AND used_at IS NULL;

-- name: CreateArcadSession :exec
INSERT INTO arcad_sessions (id, session_hash, user_id, machine_id, exposure_id, expires_at, created_at)
VALUES (
  sqlc.arg(id),
  sqlc.arg(session_hash),
  sqlc.arg(user_id),
  sqlc.arg(machine_id),
  sqlc.arg(exposure_id),
  sqlc.arg(expires_at),
  sqlc.arg(created_at)
);

-- name: GetActiveArcadSessionByHashAndMachine :one
SELECT s.id, s.user_id, u.email, s.machine_id, s.exposure_id, s.expires_at
FROM arcad_sessions s
JOIN users u ON u.id = s.user_id
WHERE s.session_hash = sqlc.arg(session_hash)
  AND s.machine_id = sqlc.arg(machine_id)
  AND s.revoked_at IS NULL
  AND s.expires_at > sqlc.arg(now_unix)
LIMIT 1;

-- name: RevokeArcadSessionByHash :exec
UPDATE arcad_sessions
SET revoked_at = sqlc.arg(revoked_at)
WHERE session_hash = sqlc.arg(session_hash)
  AND revoked_at IS NULL;

-- name: CreateSession :exec
INSERT INTO sessions (id, user_id, token_hash, expires_at)
VALUES (sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(token_hash), sqlc.arg(expires_at_unix));

-- name: GetUserByActiveSessionTokenHash :one
SELECT u.id, u.email, u.password_hash, u.password_setup_required, u.role, u.created_at
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

-- name: DeleteArcadSessionsByUserID :exec
DELETE FROM arcad_sessions
WHERE user_id = sqlc.arg(user_id);

-- name: CreateMachine :exec
INSERT INTO machines (id, name, runtime_id, setup_version)
VALUES (sqlc.arg(id), sqlc.arg(name), sqlc.arg(runtime_id), sqlc.arg(setup_version));

-- name: ListRuntimes :many
SELECT id, name, type, config_json, created_at, updated_at
FROM runtimes
ORDER BY created_at ASC;

-- name: CreateRuntime :exec
INSERT INTO runtimes (id, name, type, config_json, created_at, updated_at)
VALUES (
  sqlc.arg(id),
  sqlc.arg(name),
  sqlc.arg(type),
  sqlc.arg(config_json),
  sqlc.arg(created_at),
  sqlc.arg(updated_at)
);

-- name: GetRuntimeByID :one
SELECT id, name, type, config_json, created_at, updated_at
FROM runtimes
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: UpdateRuntimeByID :execrows
UPDATE runtimes
SET name = sqlc.arg(name),
    type = sqlc.arg(type),
    config_json = sqlc.arg(config_json),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: DeleteRuntimeByID :execrows
DELETE FROM runtimes
WHERE id = sqlc.arg(id);

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
      AND um.role = 'admin'
  );

-- name: UpdateMachineRuntimeByIDForOwner :execrows
UPDATE machines
SET runtime_id = sqlc.arg(runtime_id),
    setup_version = sqlc.arg(setup_version)
WHERE id = sqlc.arg(machine_id)
  AND EXISTS (
    SELECT 1
    FROM user_machines um
    WHERE um.machine_id = machines.id
      AND um.user_id = sqlc.arg(user_id)
      AND um.role = 'admin'
  );

-- name: DeleteUserMachineByMachineIDForOwner :execrows
DELETE FROM user_machines
WHERE machine_id = sqlc.arg(machine_id)
  AND user_id = sqlc.arg(user_id)
  AND role = 'admin';

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
INSERT INTO machine_states (machine_id, status, desired_status, ready, ready_reported_at, ready_reason, updated_at)
VALUES (
  sqlc.arg(machine_id),
  sqlc.arg(status),
  sqlc.arg(desired_status),
  FALSE,
  sqlc.arg(updated_at),
  '',
  sqlc.arg(updated_at)
);

-- name: ListMachinesAccessibleByUser :many
SELECT DISTINCT m.id, m.name, m.runtime_id, m.setup_version, m.endpoint, ms.status, ms.desired_status, ms.container_id, ms.last_error, ms.ready, ms.ready_reported_at, ms.ready_reason,
  COALESCE(um.role, '') AS user_role, m.created_at
FROM machines m
JOIN machine_states ms ON ms.machine_id = m.id
LEFT JOIN user_machines um ON um.machine_id = m.id AND um.user_id = sqlc.arg(user_id)
LEFT JOIN machine_sharing sh ON sh.machine_id = m.id
WHERE um.user_id = sqlc.arg(user_id)
  OR (sh.general_access_scope = 'arca_users' AND sh.general_access_role != 'none')
ORDER BY m.created_at DESC;

-- name: GetMachineByID :one
SELECT m.id, m.name, m.runtime_id, m.setup_version, m.endpoint, ms.status, ms.desired_status, ms.container_id, ms.last_error, ms.ready, ms.ready_reported_at, ms.ready_reason, COALESCE(mt.token, '') AS machine_token
FROM machines m
JOIN machine_states ms ON ms.machine_id = m.id
LEFT JOIN machine_tokens mt ON mt.machine_id = m.id AND mt.revoked_at IS NULL
WHERE m.id = sqlc.arg(machine_id)
LIMIT 1;

-- name: GetMachineOwnerUserID :one
SELECT um.user_id
FROM user_machines um
WHERE um.machine_id = sqlc.arg(machine_id)
  AND um.role = 'admin'
ORDER BY um.created_at ASC
LIMIT 1;

-- name: GetMachineByIDForUser :one
SELECT m.id, m.name, m.runtime_id, m.setup_version, m.endpoint, ms.status, ms.desired_status, ms.container_id, ms.last_error, ms.ready, ms.ready_reported_at, ms.ready_reason
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
    last_error = '',
    ready = FALSE,
    ready_reported_at = sqlc.arg(updated_at),
    ready_reason = 'state transition requested'
WHERE machine_states.machine_id = sqlc.arg(machine_id)
  AND EXISTS (
    SELECT 1
    FROM user_machines um
    WHERE um.machine_id = machine_states.machine_id
      AND um.user_id = sqlc.arg(user_id)
      AND um.role = 'admin'
  );

-- name: UpdateMachineRuntimeStateByMachineID :exec
UPDATE machine_states
SET status = sqlc.arg(status),
    desired_status = sqlc.arg(desired_status),
    container_id = sqlc.arg(container_id),
    last_error = sqlc.arg(last_error),
    updated_at = sqlc.arg(updated_at)
WHERE machine_id = sqlc.arg(machine_id);

-- name: ReportMachineReadinessByMachineID :execrows
UPDATE machine_states
SET ready = sqlc.arg(ready),
    ready_reported_at = sqlc.arg(ready_reported_at),
    ready_reason = sqlc.arg(ready_reason),
    container_id = CASE
      WHEN sqlc.arg(container_id) <> '' THEN sqlc.arg(container_id)
      ELSE container_id
    END,
    status = CASE
      WHEN sqlc.arg(ready) = TRUE
        AND desired_status = 'running'
        AND status IN ('pending', 'starting', 'running')
      THEN 'running'
      ELSE status
    END,
    last_error = CASE
      WHEN sqlc.arg(ready) = TRUE
        AND desired_status = 'running'
      THEN ''
      ELSE last_error
    END,
    updated_at = sqlc.arg(updated_at)
WHERE machine_id = sqlc.arg(machine_id)
  AND (
    sqlc.arg(ready) = FALSE
    OR desired_status = 'running'
  );

-- name: GetMachineReadinessByMachineID :one
SELECT ready, ready_reported_at, desired_status
FROM machine_states
WHERE machine_id = sqlc.arg(machine_id)
LIMIT 1;

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
SELECT m.id, m.name, m.runtime_id, m.setup_version, m.endpoint, ms.status, ms.desired_status, ms.container_id, ms.last_error, ms.ready, ms.ready_reported_at, ms.ready_reason
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
SELECT completed, base_domain, domain_prefix, cloudflare_api_token, updated_at
FROM setup_state
WHERE id = 1
LIMIT 1;

-- name: UpsertSetupState :exec
INSERT INTO setup_state (id, completed, base_domain, domain_prefix, cloudflare_api_token, updated_at)
VALUES (
  1,
  sqlc.arg(completed),
  sqlc.arg(base_domain),
  sqlc.arg(domain_prefix),
  sqlc.arg(cloudflare_api_token),
  sqlc.arg(updated_at)
)
ON CONFLICT (id) DO UPDATE
SET completed = excluded.completed,
    base_domain = excluded.base_domain,
    domain_prefix = excluded.domain_prefix,
    cloudflare_api_token = excluded.cloudflare_api_token,
    updated_at = excluded.updated_at;

-- name: HasAdminUser :one
SELECT COUNT(1) > 0
FROM users
WHERE role = 'admin'
LIMIT 1;

-- name: GetFirstAdminUser :one
SELECT id, email, role
FROM users
WHERE role = 'admin'
LIMIT 1;

-- name: CreateMachineToken :exec
INSERT INTO machine_tokens (id, machine_id, token_hash, token, created_at)
VALUES (sqlc.arg(id), sqlc.arg(machine_id), sqlc.arg(token_hash), sqlc.arg(token), sqlc.arg(created_at));

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
INSERT INTO machine_exposures (id, machine_id, name, hostname, service, created_at, updated_at)
VALUES (
  sqlc.arg(id),
  sqlc.arg(machine_id),
  sqlc.arg(name),
  sqlc.arg(hostname),
  sqlc.arg(service),
  sqlc.arg(created_at),
  sqlc.arg(updated_at)
)
ON CONFLICT (machine_id, name) DO UPDATE
SET hostname = excluded.hostname,
    service = excluded.service,
    updated_at = excluded.updated_at;

-- name: ListMachineExposuresByMachineID :many
SELECT id, machine_id, name, hostname, service, created_at, updated_at
FROM machine_exposures
WHERE machine_id = sqlc.arg(machine_id)
ORDER BY created_at ASC;

-- name: GetMachineExposureByHostname :one
SELECT id, machine_id, name, hostname, service, created_at, updated_at
FROM machine_exposures
WHERE hostname = sqlc.arg(hostname)
LIMIT 1;

-- name: GetMachineExposureByMachineIDAndName :one
SELECT id, machine_id, name, hostname, service, created_at, updated_at
FROM machine_exposures
WHERE machine_id = sqlc.arg(machine_id)
  AND name = sqlc.arg(name)
LIMIT 1;

-- name: GetMachineSharingByMachineID :one
SELECT machine_id, general_access_scope, general_access_role, updated_at
FROM machine_sharing
WHERE machine_id = sqlc.arg(machine_id)
LIMIT 1;

-- name: UpsertMachineSharing :exec
INSERT INTO machine_sharing (machine_id, general_access_scope, general_access_role, updated_at)
VALUES (
  sqlc.arg(machine_id),
  sqlc.arg(general_access_scope),
  sqlc.arg(general_access_role),
  sqlc.arg(updated_at)
)
ON CONFLICT (machine_id) DO UPDATE
SET general_access_scope = excluded.general_access_scope,
    general_access_role = excluded.general_access_role,
    updated_at = excluded.updated_at;

-- name: ListUserMachinesByMachineID :many
SELECT um.user_id, um.machine_id, um.role, u.email
FROM user_machines um
JOIN users u ON u.id = um.user_id
WHERE um.machine_id = sqlc.arg(machine_id)
ORDER BY um.created_at ASC;

-- name: UpsertUserMachine :exec
INSERT INTO user_machines (user_id, machine_id, role)
VALUES (sqlc.arg(user_id), sqlc.arg(machine_id), sqlc.arg(role))
ON CONFLICT (user_id, machine_id) DO UPDATE
SET role = excluded.role;

-- name: DeleteUserMachine :execrows
DELETE FROM user_machines
WHERE user_id = sqlc.arg(user_id)
  AND machine_id = sqlc.arg(machine_id);

-- name: GetUserMachineRole :one
SELECT role
FROM user_machines
WHERE user_id = sqlc.arg(user_id)
  AND machine_id = sqlc.arg(machine_id)
LIMIT 1;

-- name: ListMachineEventsByMachineID :many
SELECT id, machine_id, job_id, level, event_type, message, created_at
FROM machine_events
WHERE machine_id = sqlc.arg(machine_id)
ORDER BY created_at DESC
LIMIT sqlc.arg(limit_n);
