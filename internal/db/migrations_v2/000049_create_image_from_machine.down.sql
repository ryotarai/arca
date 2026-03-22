ALTER TABLE machine_states DROP COLUMN locked_operation;
ALTER TABLE custom_images DROP COLUMN source_machine_id;

-- Recreate machine_jobs without new columns and with old CHECK
CREATE TABLE machine_jobs_old (
  id TEXT PRIMARY KEY,
  machine_id TEXT NOT NULL REFERENCES machines(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('start', 'stop', 'delete', 'reconcile')),
  status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
  attempt INTEGER NOT NULL DEFAULT 0,
  next_run_at BIGINT NOT NULL,
  lease_owner TEXT,
  lease_until BIGINT,
  last_error TEXT,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL
);

INSERT INTO machine_jobs_old SELECT id, machine_id, kind, status, attempt, next_run_at, lease_owner, lease_until, last_error, created_at, updated_at FROM machine_jobs WHERE kind != 'create_image';

DROP TABLE machine_jobs;
ALTER TABLE machine_jobs_old RENAME TO machine_jobs;

CREATE INDEX IF NOT EXISTS idx_machine_jobs_status_next_run_at
  ON machine_jobs(status, next_run_at);
CREATE INDEX IF NOT EXISTS idx_machine_jobs_lease_until
  ON machine_jobs(status, lease_until);
CREATE INDEX IF NOT EXISTS idx_machine_jobs_machine_id_status
  ON machine_jobs(machine_id, status);
