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

-- Drop exposure visibility columns and ACL table
-- SQLite doesn't support DROP COLUMN before 3.35.0, so we recreate the table.

CREATE TABLE machine_exposures_new (
  id TEXT PRIMARY KEY,
  machine_id TEXT NOT NULL REFERENCES machines(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  hostname TEXT NOT NULL UNIQUE,
  service TEXT NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  UNIQUE(machine_id, name)
);

INSERT INTO machine_exposures_new (id, machine_id, name, hostname, service, created_at, updated_at)
SELECT id, machine_id, name, hostname, service, created_at, updated_at FROM machine_exposures;

DROP TABLE IF EXISTS machine_exposure_acl_users;
DROP TABLE IF EXISTS machine_exposures;
ALTER TABLE machine_exposures_new RENAME TO machine_exposures;

CREATE INDEX IF NOT EXISTS idx_machine_exposures_machine_id ON machine_exposures(machine_id);
