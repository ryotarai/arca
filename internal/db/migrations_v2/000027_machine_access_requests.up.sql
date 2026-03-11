CREATE TABLE IF NOT EXISTS machine_access_requests (
  id TEXT PRIMARY KEY,
  machine_id TEXT NOT NULL REFERENCES machines(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  status TEXT NOT NULL DEFAULT 'pending',
  requested_role TEXT NOT NULL DEFAULT 'viewer',
  resolved_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
  resolved_role TEXT NOT NULL DEFAULT '',
  message TEXT NOT NULL DEFAULT '',
  created_at BIGINT NOT NULL,
  resolved_at BIGINT
);
CREATE INDEX IF NOT EXISTS idx_machine_access_requests_machine_status
  ON machine_access_requests(machine_id, status);
