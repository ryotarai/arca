CREATE TABLE IF NOT EXISTS user_groups (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS user_group_members (
  group_id TEXT NOT NULL REFERENCES user_groups(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  PRIMARY KEY (group_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_user_group_members_user_id ON user_group_members(user_id);

CREATE TABLE IF NOT EXISTS machine_group_access (
  machine_id TEXT NOT NULL REFERENCES machines(id) ON DELETE CASCADE,
  group_id TEXT NOT NULL REFERENCES user_groups(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'viewer',
  PRIMARY KEY (machine_id, group_id)
);
CREATE INDEX IF NOT EXISTS idx_machine_group_access_group_id ON machine_group_access(group_id);
