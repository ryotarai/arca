-- Rename owner role to admin in user_machines
UPDATE user_machines SET role = 'admin' WHERE role = 'owner';

-- Create machine_sharing table for general access settings
CREATE TABLE IF NOT EXISTS machine_sharing (
  machine_id TEXT PRIMARY KEY REFERENCES machines(id) ON DELETE CASCADE,
  general_access_scope TEXT NOT NULL DEFAULT 'none',
  general_access_role TEXT NOT NULL DEFAULT 'none',
  updated_at BIGINT NOT NULL DEFAULT 0
);

-- Backfill machine_sharing for all existing machines
INSERT INTO machine_sharing (machine_id, general_access_scope, general_access_role, updated_at)
SELECT id, 'none', 'none', 0 FROM machines
WHERE id NOT IN (SELECT machine_id FROM machine_sharing);

-- Drop ACL table (no longer used)
DROP TABLE IF EXISTS machine_exposure_acl_users;

-- The visibility and is_public columns on machine_exposures are no longer
-- read or written by the application. SQLite cannot reliably DROP COLUMN
-- when the table has foreign key references, so we leave them in place as
-- harmless no-ops (same approach as migration 000024).
