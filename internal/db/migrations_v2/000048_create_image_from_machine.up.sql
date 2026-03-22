-- Add locked_operation to machine_states
ALTER TABLE machine_states ADD COLUMN locked_operation TEXT;

-- Add source_machine_id to custom_images
ALTER TABLE custom_images ADD COLUMN source_machine_id TEXT;

-- Recreate machine_jobs to update CHECK constraint on kind and add new columns
CREATE TABLE machine_jobs_new (
  id TEXT PRIMARY KEY,
  machine_id TEXT NOT NULL REFERENCES machines(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('start', 'stop', 'delete', 'reconcile', 'create_image')),
  status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
  attempt INTEGER NOT NULL DEFAULT 0,
  next_run_at BIGINT NOT NULL,
  lease_owner TEXT,
  lease_until BIGINT,
  last_error TEXT,
  description TEXT,
  metadata_json TEXT,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL
);

INSERT INTO machine_jobs_new (id, machine_id, kind, status, attempt, next_run_at, lease_owner, lease_until, last_error, created_at, updated_at)
SELECT id, machine_id, kind, status, attempt, next_run_at, lease_owner, lease_until, last_error, created_at, updated_at FROM machine_jobs;

DROP TABLE machine_jobs;
ALTER TABLE machine_jobs_new RENAME TO machine_jobs;

-- Recreate indexes dropped with the old table
CREATE INDEX IF NOT EXISTS idx_machine_jobs_status_next_run_at
  ON machine_jobs(status, next_run_at);
CREATE INDEX IF NOT EXISTS idx_machine_jobs_lease_until
  ON machine_jobs(status, lease_until);
CREATE INDEX IF NOT EXISTS idx_machine_jobs_machine_id_status
  ON machine_jobs(machine_id, status);
