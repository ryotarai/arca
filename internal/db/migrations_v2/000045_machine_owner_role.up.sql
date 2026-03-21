-- Add 'owner' to the allowed roles for user_machines.
-- SQLite does not support ALTER CONSTRAINT, so we recreate the table.

CREATE TABLE user_machines_new (
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  machine_id TEXT NOT NULL REFERENCES machines(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'admin' CHECK (role IN ('owner', 'admin', 'editor', 'viewer')),
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, machine_id)
);

INSERT INTO user_machines_new (user_id, machine_id, role, created_at)
SELECT user_id, machine_id, role, created_at
FROM user_machines;

DROP TABLE user_machines;
ALTER TABLE user_machines_new RENAME TO user_machines;

CREATE INDEX IF NOT EXISTS idx_user_machines_machine_id ON user_machines(machine_id);

-- Promote the earliest admin per machine to owner.
UPDATE user_machines
SET role = 'owner'
WHERE rowid IN (
  SELECT MIN(sub.rowid)
  FROM user_machines sub
  WHERE sub.role = 'admin'
  GROUP BY sub.machine_id
);
