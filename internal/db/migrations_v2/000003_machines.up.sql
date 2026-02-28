CREATE TABLE IF NOT EXISTS machines (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS user_machines (
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  machine_id TEXT NOT NULL REFERENCES machines(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'owner',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, machine_id)
);

CREATE INDEX IF NOT EXISTS idx_user_machines_machine_id ON user_machines(machine_id);
