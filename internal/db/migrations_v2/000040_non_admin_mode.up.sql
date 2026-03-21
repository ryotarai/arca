CREATE TABLE IF NOT EXISTS admin_view_mode (
    user_id TEXT PRIMARY KEY NOT NULL,
    mode TEXT NOT NULL DEFAULT 'admin',
    updated_at BIGINT NOT NULL
);

ALTER TABLE sessions DROP COLUMN impersonated_user_id;
ALTER TABLE sessions DROP COLUMN impersonated_by_user_id;
