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
SELECT id, email, password_hash, password_setup_required, role, startup_script, agent_prompt, created_at
FROM users
ORDER BY created_at DESC;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, password_setup_required, role, startup_script, agent_prompt, created_at
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
SELECT id, email, password_hash, password_setup_required, role, startup_script, agent_prompt, created_at
FROM users
WHERE id = sqlc.arg(id)
LIMIT 1;

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

-- name: GetUserStartupScript :one
SELECT startup_script
FROM users
WHERE id = sqlc.arg(user_id);

-- name: UpdateUserStartupScript :exec
UPDATE users
SET startup_script = sqlc.arg(startup_script)
WHERE id = sqlc.arg(user_id);

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
SELECT u.id, u.email, u.password_hash, u.password_setup_required, u.role, u.startup_script, u.agent_prompt, u.created_at
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
INSERT INTO machines (id, name, template_id, template_type, template_config_json, setup_version, options_json, custom_image_id)
VALUES (sqlc.arg(id), sqlc.arg(name), sqlc.arg(template_id), sqlc.arg(template_type), sqlc.arg(template_config_json), sqlc.arg(setup_version), sqlc.arg(options_json), sqlc.arg(custom_image_id));

-- name: ListMachineTemplates :many
SELECT id, name, type, config_json, created_at, updated_at
FROM machine_templates
ORDER BY created_at ASC;

-- name: CreateMachineTemplate :exec
INSERT INTO machine_templates (id, name, type, config_json, created_at, updated_at)
VALUES (
  sqlc.arg(id),
  sqlc.arg(name),
  sqlc.arg(type),
  sqlc.arg(config_json),
  sqlc.arg(created_at),
  sqlc.arg(updated_at)
);

-- name: GetMachineTemplateByID :one
SELECT id, name, type, config_json, created_at, updated_at
FROM machine_templates
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: UpdateMachineTemplateByID :execrows
UPDATE machine_templates
SET name = sqlc.arg(name),
    type = sqlc.arg(type),
    config_json = sqlc.arg(config_json),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: DeleteMachineTemplateByID :execrows
DELETE FROM machine_templates
WHERE id = sqlc.arg(id);

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
      AND um.role IN ('owner', 'admin')
  );

-- name: DeleteUserMachineByMachineIDForOwner :execrows
DELETE FROM user_machines
WHERE machine_id = sqlc.arg(machine_id)
  AND user_id = sqlc.arg(user_id)
  AND role IN ('owner', 'admin');

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
SELECT DISTINCT m.id, m.name, m.template_id, m.template_type, m.template_config_json, m.setup_version, m.options_json, m.custom_image_id, ms.status, ms.desired_status, ms.container_id, ms.last_error, ms.ready, ms.ready_reported_at, ms.ready_reason, ms.arcad_version,
  COALESCE(um.role, '') AS user_role, m.created_at
FROM machines m
JOIN machine_states ms ON ms.machine_id = m.id
LEFT JOIN user_machines um ON um.machine_id = m.id AND um.user_id = sqlc.arg(user_id)
LEFT JOIN machine_sharing sh ON sh.machine_id = m.id
LEFT JOIN machine_group_access mga ON mga.machine_id = m.id
LEFT JOIN user_group_members ugm ON ugm.group_id = mga.group_id AND ugm.user_id = sqlc.arg(user_id)
WHERE um.user_id = sqlc.arg(user_id)
  OR (sh.general_access_scope = 'arca_users' AND sh.general_access_role != 'none')
  OR ugm.user_id = sqlc.arg(user_id)
ORDER BY m.created_at DESC;

-- name: GetMachineByID :one
SELECT m.id, m.name, m.template_id, m.template_type, m.template_config_json, m.setup_version, m.options_json, m.custom_image_id, ms.status, ms.desired_status, ms.container_id, ms.last_error, ms.ready, ms.ready_reported_at, ms.ready_reason, ms.arcad_version, COALESCE(mt.token, '') AS machine_token
FROM machines m
JOIN machine_states ms ON ms.machine_id = m.id
LEFT JOIN machine_tokens mt ON mt.machine_id = m.id AND mt.revoked_at IS NULL
WHERE m.id = sqlc.arg(machine_id)
LIMIT 1;

-- name: GetMachineOwnerUserID :one
SELECT um.user_id
FROM user_machines um
WHERE um.machine_id = sqlc.arg(machine_id)
  AND um.role = 'owner'
LIMIT 1;

-- name: GetMachineByIDForUser :one
SELECT m.id, m.name, m.template_id, m.template_type, m.template_config_json, m.setup_version, m.options_json, m.custom_image_id, ms.status, ms.desired_status, ms.container_id, ms.last_error, ms.ready, ms.ready_reported_at, ms.ready_reason, ms.arcad_version
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
      AND um.role IN ('owner', 'admin')
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
    arcad_version = CASE
      WHEN sqlc.arg(arcad_version) <> '' THEN sqlc.arg(arcad_version)
      ELSE arcad_version
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
SELECT mj.id, mj.machine_id, mj.kind, mj.attempt
FROM machine_jobs mj
WHERE mj.status = 'queued'
  AND mj.next_run_at <= sqlc.arg(now_unix)
  AND mj.machine_id NOT IN (
    SELECT mj2.machine_id FROM machine_jobs mj2 WHERE mj2.status = 'running'
  )
ORDER BY mj.created_at ASC
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

-- name: MarkMachineJobFailed :exec
UPDATE machine_jobs
SET status = 'failed',
    lease_owner = NULL,
    lease_until = NULL,
    last_error = sqlc.arg(last_error),
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

-- name: ExtendMachineJobLease :execrows
UPDATE machine_jobs
SET lease_until = sqlc.arg(lease_until), updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id) AND status = 'running' AND lease_owner = sqlc.arg(lease_owner);

-- name: ListMachinesByDesiredStatus :many
SELECT m.id, m.name, m.template_id, m.template_type, m.template_config_json, m.setup_version, m.options_json, m.custom_image_id, ms.status, ms.desired_status, ms.container_id, ms.last_error, ms.ready, ms.ready_reported_at, ms.ready_reason, ms.arcad_version, ms.last_activity_at
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
SELECT completed, base_domain, domain_prefix, updated_at
FROM setup_state
WHERE id = 1
LIMIT 1;

-- name: UpsertSetupState :exec
INSERT INTO setup_state (id, completed, base_domain, domain_prefix, updated_at)
VALUES (
  1,
  sqlc.arg(completed),
  sqlc.arg(base_domain),
  sqlc.arg(domain_prefix),
  sqlc.arg(updated_at)
)
ON CONFLICT (id) DO UPDATE
SET completed = excluded.completed,
    base_domain = excluded.base_domain,
    domain_prefix = excluded.domain_prefix,
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

-- name: GetMachineByName :one
SELECT m.id, m.name, m.template_id, m.template_type, m.template_config_json, m.setup_version, m.options_json, m.custom_image_id, ms.status, ms.desired_status, ms.container_id, ms.last_error, ms.ready, ms.ready_reported_at, ms.ready_reason, ms.arcad_version
FROM machines m
JOIN machine_states ms ON ms.machine_id = m.id
WHERE m.name = sqlc.arg(name)
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

-- name: UpdateMachineLastActivityAt :exec
UPDATE machine_states
SET last_activity_at = sqlc.arg(last_activity_at)
WHERE machine_id = sqlc.arg(machine_id)
  AND last_activity_at < sqlc.arg(last_activity_at);

-- name: RequestSystemStopMachine :execrows
UPDATE machine_states
SET status = 'stopping',
    desired_status = 'stopped',
    updated_at = sqlc.arg(updated_at)
WHERE machine_id = sqlc.arg(machine_id)
  AND desired_status = 'running'
  AND status = 'running';

-- name: CreateMachineAccessRequest :exec
INSERT INTO machine_access_requests (id, machine_id, user_id, status, requested_role, message, created_at)
VALUES (
  sqlc.arg(id),
  sqlc.arg(machine_id),
  sqlc.arg(user_id),
  'pending',
  sqlc.arg(requested_role),
  sqlc.arg(message),
  sqlc.arg(created_at)
);

-- name: GetPendingMachineAccessRequest :one
SELECT id, machine_id, user_id, status, requested_role, message, created_at
FROM machine_access_requests
WHERE machine_id = sqlc.arg(machine_id)
  AND user_id = sqlc.arg(user_id)
  AND status = 'pending'
LIMIT 1;

-- name: ListPendingMachineAccessRequestsByMachineID :many
SELECT r.id, r.machine_id, r.user_id, r.status, r.requested_role, r.message, r.created_at, u.email
FROM machine_access_requests r
JOIN users u ON u.id = r.user_id
WHERE r.machine_id = sqlc.arg(machine_id)
  AND r.status = 'pending'
ORDER BY r.created_at ASC;

-- name: ResolveMachineAccessRequest :execrows
UPDATE machine_access_requests
SET status = sqlc.arg(status),
    resolved_by_user_id = sqlc.arg(resolved_by_user_id),
    resolved_role = sqlc.arg(resolved_role),
    resolved_at = sqlc.arg(resolved_at)
WHERE id = sqlc.arg(id)
  AND status = 'pending';

-- name: GetMachineAccessRequestByID :one
SELECT id, machine_id, user_id, status, requested_role, message, created_at, resolved_at
FROM machine_access_requests
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: GetUserNotificationSettings :one
SELECT user_id, slack_enabled, slack_user_id, created_at, updated_at
FROM user_notification_settings
WHERE user_id = sqlc.arg(user_id)
LIMIT 1;

-- name: UpsertUserNotificationSettings :exec
INSERT INTO user_notification_settings (user_id, slack_enabled, slack_user_id, created_at, updated_at)
VALUES (
  sqlc.arg(user_id),
  sqlc.arg(slack_enabled),
  sqlc.arg(slack_user_id),
  sqlc.arg(created_at),
  sqlc.arg(updated_at)
)
ON CONFLICT (user_id) DO UPDATE
SET slack_enabled = excluded.slack_enabled,
    slack_user_id = excluded.slack_user_id,
    updated_at = excluded.updated_at;

-- name: ListNotificationEnabledUsers :many
SELECT user_id, slack_enabled, slack_user_id
FROM user_notification_settings
WHERE slack_enabled = true
  AND slack_user_id != '';

-- name: ListUserGroups :many
SELECT g.id, g.name, g.created_at,
  (SELECT COUNT(*) FROM user_group_members m WHERE m.group_id = g.id) AS member_count
FROM user_groups g
ORDER BY g.name ASC;

-- name: GetUserGroup :one
SELECT id, name, created_at
FROM user_groups
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: CreateUserGroup :exec
INSERT INTO user_groups (id, name)
VALUES (sqlc.arg(id), sqlc.arg(name));

-- name: DeleteUserGroup :execrows
DELETE FROM user_groups
WHERE id = sqlc.arg(id);

-- name: ListUserGroupMembers :many
SELECT m.group_id, m.user_id, u.email
FROM user_group_members m
JOIN users u ON u.id = m.user_id
WHERE m.group_id = sqlc.arg(group_id)
ORDER BY u.email ASC;

-- name: AddUserGroupMember :exec
INSERT INTO user_group_members (group_id, user_id)
VALUES (sqlc.arg(group_id), sqlc.arg(user_id))
ON CONFLICT (group_id, user_id) DO NOTHING;

-- name: RemoveUserGroupMember :execrows
DELETE FROM user_group_members
WHERE group_id = sqlc.arg(group_id)
  AND user_id = sqlc.arg(user_id);

-- name: ListUserGroupsByUserID :many
SELECT g.id, g.name
FROM user_groups g
JOIN user_group_members m ON m.group_id = g.id
WHERE m.user_id = sqlc.arg(user_id)
ORDER BY g.name ASC;

-- name: ListMachineGroupAccess :many
SELECT mga.machine_id, mga.group_id, mga.role, g.name AS group_name
FROM machine_group_access mga
JOIN user_groups g ON g.id = mga.group_id
WHERE mga.machine_id = sqlc.arg(machine_id)
ORDER BY g.name ASC;

-- name: UpsertMachineGroupAccess :exec
INSERT INTO machine_group_access (machine_id, group_id, role)
VALUES (sqlc.arg(machine_id), sqlc.arg(group_id), sqlc.arg(role))
ON CONFLICT (machine_id, group_id) DO UPDATE
SET role = excluded.role;

-- name: DeleteMachineGroupAccess :execrows
DELETE FROM machine_group_access
WHERE machine_id = sqlc.arg(machine_id)
  AND group_id = sqlc.arg(group_id);

-- name: GetMachineGroupRoleByUserID :one
SELECT mga.role
FROM machine_group_access mga
JOIN user_group_members ugm ON ugm.group_id = mga.group_id
WHERE mga.machine_id = sqlc.arg(machine_id)
  AND ugm.user_id = sqlc.arg(user_id)
ORDER BY CASE mga.role
  WHEN 'admin' THEN 1
  WHEN 'editor' THEN 2
  WHEN 'viewer' THEN 3
  ELSE 4
END ASC
LIMIT 1;

-- name: SearchUserGroups :many
SELECT id, name
FROM user_groups
WHERE LOWER(name) LIKE '%' || LOWER(sqlc.arg(query)) || '%'
ORDER BY name ASC
LIMIT sqlc.arg(limit_count);

-- name: UpdateMachineOptionsByID :execrows
UPDATE machines
SET options_json = sqlc.arg(options_json)
WHERE id = sqlc.arg(machine_id);

-- name: ListUserLLMModels :many
SELECT id, user_id, config_name, endpoint_type, custom_endpoint, model_name, max_context_tokens, created_at, updated_at
FROM user_llm_models
WHERE user_id = sqlc.arg(user_id)
ORDER BY created_at ASC;

-- name: GetUserLLMModel :one
SELECT id, user_id, config_name, endpoint_type, custom_endpoint, model_name, api_key_encrypted, max_context_tokens, created_at, updated_at
FROM user_llm_models
WHERE id = sqlc.arg(id)
  AND user_id = sqlc.arg(user_id)
LIMIT 1;

-- name: CreateUserLLMModel :exec
INSERT INTO user_llm_models (id, user_id, config_name, endpoint_type, custom_endpoint, model_name, api_key_encrypted, max_context_tokens, created_at, updated_at)
VALUES (
  sqlc.arg(id),
  sqlc.arg(user_id),
  sqlc.arg(config_name),
  sqlc.arg(endpoint_type),
  sqlc.arg(custom_endpoint),
  sqlc.arg(model_name),
  sqlc.arg(api_key_encrypted),
  sqlc.arg(max_context_tokens),
  sqlc.arg(created_at),
  sqlc.arg(updated_at)
);

-- name: UpdateUserLLMModel :execrows
UPDATE user_llm_models
SET config_name = sqlc.arg(config_name),
    endpoint_type = sqlc.arg(endpoint_type),
    custom_endpoint = sqlc.arg(custom_endpoint),
    model_name = sqlc.arg(model_name),
    api_key_encrypted = sqlc.arg(api_key_encrypted),
    max_context_tokens = sqlc.arg(max_context_tokens),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id)
  AND user_id = sqlc.arg(user_id);

-- name: DeleteUserLLMModel :execrows
DELETE FROM user_llm_models
WHERE id = sqlc.arg(id)
  AND user_id = sqlc.arg(user_id);

-- name: ListUserLLMModelsWithAPIKey :many
SELECT id, user_id, config_name, endpoint_type, custom_endpoint, model_name, api_key_encrypted, max_context_tokens, created_at, updated_at
FROM user_llm_models
WHERE user_id = sqlc.arg(user_id)
ORDER BY created_at ASC;

-- name: ListAllUserLLMModelsEncryptedKeys :many
SELECT id, api_key_encrypted
FROM user_llm_models
WHERE api_key_encrypted != ''
ORDER BY id ASC;

-- name: UpdateUserLLMModelEncryptedKey :execrows
UPDATE user_llm_models
SET api_key_encrypted = sqlc.arg(api_key_encrypted)
WHERE id = sqlc.arg(id);

-- name: UpsertAdminViewMode :exec
INSERT INTO admin_view_mode (user_id, mode, updated_at)
VALUES (sqlc.arg(user_id), sqlc.arg(mode), sqlc.arg(updated_at))
ON CONFLICT(user_id) DO UPDATE SET mode = excluded.mode, updated_at = excluded.updated_at;

-- name: GetAdminViewMode :one
SELECT mode FROM admin_view_mode WHERE user_id = sqlc.arg(user_id);

-- name: CreateAuditLog :exec
INSERT INTO audit_logs (id, actor_user_id, acting_as_user_id, action, resource_type, resource_id, details_json, created_at)
VALUES (
  sqlc.arg(id),
  sqlc.arg(actor_user_id),
  sqlc.narg(acting_as_user_id),
  sqlc.arg(action),
  sqlc.arg(resource_type),
  sqlc.arg(resource_id),
  sqlc.arg(details_json),
  sqlc.arg(created_at)
);

-- name: ListAuditLogs :many
SELECT al.id, al.actor_user_id, al.acting_as_user_id, al.action, al.resource_type, al.resource_id, al.details_json, al.created_at,
  u1.email AS actor_email,
  u2.email AS acting_as_email
FROM audit_logs al
JOIN users u1 ON u1.id = al.actor_user_id
LEFT JOIN users u2 ON u2.id = al.acting_as_user_id
ORDER BY al.created_at DESC
LIMIT sqlc.arg(limit_count);

-- name: ListAuditLogsFiltered :many
SELECT al.id, al.actor_user_id, al.acting_as_user_id, al.action, al.resource_type, al.resource_id, al.details_json, al.created_at,
  u1.email AS actor_email,
  u2.email AS acting_as_email
FROM audit_logs al
JOIN users u1 ON u1.id = al.actor_user_id
LEFT JOIN users u2 ON u2.id = al.acting_as_user_id
WHERE (sqlc.arg(action_prefix) = '' OR al.action LIKE sqlc.arg(action_prefix) || '%')
  AND (sqlc.arg(actor_email) = '' OR u1.email = sqlc.arg(actor_email))
ORDER BY al.created_at DESC
LIMIT sqlc.arg(limit_count) OFFSET sqlc.arg(offset_count);

-- name: CountAuditLogsFiltered :one
SELECT COUNT(*) AS total_count
FROM audit_logs al
JOIN users u1 ON u1.id = al.actor_user_id
WHERE (sqlc.arg(action_prefix) = '' OR al.action LIKE sqlc.arg(action_prefix) || '%')
  AND (sqlc.arg(actor_email) = '' OR u1.email = sqlc.arg(actor_email));

-- name: ListCustomImages :many
SELECT id, name, template_type, data_json, description, created_at, updated_at
FROM custom_images
ORDER BY created_at DESC;

-- name: ListCustomImagesByRuntimeType :many
SELECT id, name, template_type, data_json, description, created_at, updated_at
FROM custom_images
WHERE template_type = sqlc.arg(template_type)
ORDER BY created_at DESC;

-- name: GetCustomImage :one
SELECT id, name, template_type, data_json, description, created_at, updated_at
FROM custom_images
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: CreateCustomImage :exec
INSERT INTO custom_images (id, name, template_type, data_json, description, created_at, updated_at)
VALUES (
  sqlc.arg(id),
  sqlc.arg(name),
  sqlc.arg(template_type),
  sqlc.arg(data_json),
  sqlc.arg(description),
  sqlc.arg(created_at),
  sqlc.arg(updated_at)
);

-- name: UpdateCustomImage :execrows
UPDATE custom_images
SET name = sqlc.arg(name),
    template_type = sqlc.arg(template_type),
    data_json = sqlc.arg(data_json),
    description = sqlc.arg(description),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: DeleteCustomImage :execrows
DELETE FROM custom_images
WHERE id = sqlc.arg(id);

-- name: ListCustomImagesByTemplateID :many
SELECT ci.id, ci.name, ci.template_type, ci.data_json, ci.description, ci.created_at, ci.updated_at
FROM custom_images ci
JOIN template_custom_images tci ON tci.custom_image_id = ci.id
WHERE tci.template_id = sqlc.arg(template_id)
ORDER BY ci.name ASC;

-- name: AssociateTemplateCustomImage :exec
INSERT INTO template_custom_images (template_id, custom_image_id)
VALUES (sqlc.arg(template_id), sqlc.arg(custom_image_id))
ON CONFLICT (template_id, custom_image_id) DO NOTHING;

-- name: DisassociateTemplateCustomImage :execrows
DELETE FROM template_custom_images
WHERE template_id = sqlc.arg(template_id)
  AND custom_image_id = sqlc.arg(custom_image_id);

-- name: DisassociateAllTemplatesFromCustomImage :exec
DELETE FROM template_custom_images
WHERE custom_image_id = sqlc.arg(custom_image_id);

-- name: ListTemplateIDsByCustomImageID :many
SELECT template_id
FROM template_custom_images
WHERE custom_image_id = sqlc.arg(custom_image_id)
ORDER BY template_id ASC;

-- name: ListServerLLMModels :many
SELECT id, config_name, endpoint_type, custom_endpoint, model_name, token_command, max_context_tokens, created_at, updated_at
FROM server_llm_models
ORDER BY created_at ASC;

-- name: GetServerLLMModel :one
SELECT id, config_name, endpoint_type, custom_endpoint, model_name, token_command, max_context_tokens, created_at, updated_at
FROM server_llm_models
WHERE id = sqlc.arg(id)
LIMIT 1;

-- name: CreateServerLLMModel :exec
INSERT INTO server_llm_models (id, config_name, endpoint_type, custom_endpoint, model_name, token_command, max_context_tokens, created_at, updated_at)
VALUES (
  sqlc.arg(id),
  sqlc.arg(config_name),
  sqlc.arg(endpoint_type),
  sqlc.arg(custom_endpoint),
  sqlc.arg(model_name),
  sqlc.arg(token_command),
  sqlc.arg(max_context_tokens),
  sqlc.arg(created_at),
  sqlc.arg(updated_at)
);

-- name: UpdateServerLLMModel :execrows
UPDATE server_llm_models
SET config_name = sqlc.arg(config_name),
    endpoint_type = sqlc.arg(endpoint_type),
    custom_endpoint = sqlc.arg(custom_endpoint),
    model_name = sqlc.arg(model_name),
    token_command = sqlc.arg(token_command),
    max_context_tokens = sqlc.arg(max_context_tokens),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: DeleteServerLLMModel :execrows
DELETE FROM server_llm_models
WHERE id = sqlc.arg(id);

-- name: InsertRateLimitEntry :exec
INSERT INTO rate_limit_entries (key, timestamp_unix) VALUES (sqlc.arg(key), sqlc.arg(timestamp_unix));

-- name: CountRateLimitEntries :one
SELECT COUNT(*) FROM rate_limit_entries WHERE key = sqlc.arg(key) AND timestamp_unix > sqlc.arg(window_start);

-- name: CleanupRateLimitEntries :exec
DELETE FROM rate_limit_entries WHERE timestamp_unix < sqlc.arg(cutoff);

-- name: ListMachineTagsByMachineID :many
SELECT tag FROM machine_tags WHERE machine_id = sqlc.arg(machine_id) ORDER BY tag;

-- name: DeleteMachineTagsByMachineID :exec
DELETE FROM machine_tags WHERE machine_id = sqlc.arg(machine_id);

-- name: InsertMachineTag :exec
INSERT INTO machine_tags (machine_id, tag) VALUES (sqlc.arg(machine_id), sqlc.arg(tag))
ON CONFLICT (machine_id, tag) DO NOTHING;

-- name: ListAllMachineTags :many
SELECT machine_id, tag FROM machine_tags ORDER BY machine_id, tag;

-- name: CountRecentJobsByStatus :one
SELECT
    COALESCE(SUM(CASE WHEN status = 'succeeded' AND updated_at > sqlc.arg(since) THEN 1 ELSE 0 END), 0) as succeeded,
    COALESCE(SUM(CASE WHEN status = 'failed' AND updated_at > sqlc.arg(since) THEN 1 ELSE 0 END), 0) as failed,
    COALESCE(SUM(CASE WHEN status = 'running' AND lease_until < sqlc.arg(now) THEN 1 ELSE 0 END), 0) as stuck
FROM machine_jobs;

-- name: GetUserAgentPromptByID :one
SELECT agent_prompt FROM users WHERE id = sqlc.arg(id) LIMIT 1;

-- name: UpdateUserAgentPromptByID :execrows
UPDATE users SET agent_prompt = sqlc.arg(agent_prompt) WHERE id = sqlc.arg(id);
