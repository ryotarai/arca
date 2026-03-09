ALTER TABLE machine_states
  ADD COLUMN ready BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE machine_states
  ADD COLUMN ready_reported_at BIGINT NOT NULL DEFAULT 0;

ALTER TABLE machine_states
  ADD COLUMN ready_reason TEXT NOT NULL DEFAULT '';
