ALTER TABLE machine_events RENAME TO machine_events_old;

CREATE TABLE machine_events (
  id TEXT PRIMARY KEY,
  machine_id TEXT NOT NULL,
  job_id TEXT NOT NULL DEFAULT '',
  level TEXT NOT NULL DEFAULT 'info',
  event_type TEXT NOT NULL,
  message TEXT NOT NULL,
  created_at BIGINT NOT NULL
);

INSERT INTO machine_events (id, machine_id, job_id, level, event_type, message, created_at)
SELECT id, machine_id, job_id, level, event_type, message, created_at
FROM machine_events_old;

DROP TABLE machine_events_old;

CREATE INDEX IF NOT EXISTS idx_machine_events_machine_id_created_at
  ON machine_events(machine_id, created_at DESC);
