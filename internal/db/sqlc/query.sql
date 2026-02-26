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

-- name: ListMachinesByUser :many
SELECT m.id, m.name, m.created_at
FROM machines m
JOIN user_machines um ON um.machine_id = m.id
WHERE um.user_id = sqlc.arg(user_id)
ORDER BY m.created_at DESC;

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
