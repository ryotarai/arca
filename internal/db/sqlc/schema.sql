CREATE TABLE IF NOT EXISTS app_meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  password_setup_required BOOLEAN NOT NULL DEFAULT FALSE,
  role TEXT NOT NULL DEFAULT 'user',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS user_setup_tokens (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
  expires_at BIGINT NOT NULL,
  used_at BIGINT,
  created_at BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_user_setup_tokens_user_id ON user_setup_tokens(user_id);

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  expires_at BIGINT NOT NULL,
  revoked_at TIMESTAMP,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS arcad_exchange_tokens (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  machine_id TEXT NOT NULL REFERENCES machines(id) ON DELETE CASCADE,
  exposure_id TEXT NOT NULL DEFAULT '',
  expires_at BIGINT NOT NULL,
  used_at BIGINT,
  created_at BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_arcad_exchange_tokens_machine_id ON arcad_exchange_tokens(machine_id);
CREATE INDEX IF NOT EXISTS idx_arcad_exchange_tokens_expires_at ON arcad_exchange_tokens(expires_at);

CREATE TABLE IF NOT EXISTS arcad_sessions (
  id TEXT PRIMARY KEY,
  session_hash TEXT NOT NULL UNIQUE,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  machine_id TEXT NOT NULL REFERENCES machines(id) ON DELETE CASCADE,
  exposure_id TEXT NOT NULL DEFAULT '',
  expires_at BIGINT NOT NULL,
  revoked_at BIGINT,
  created_at BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_arcad_sessions_machine_id ON arcad_sessions(machine_id);
CREATE INDEX IF NOT EXISTS idx_arcad_sessions_user_id ON arcad_sessions(user_id);

CREATE TABLE IF NOT EXISTS machines (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  runtime_id TEXT NOT NULL DEFAULT 'libvirt',
  setup_version TEXT NOT NULL DEFAULT '',
  endpoint TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS runtimes (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  type TEXT NOT NULL,
  config_json TEXT NOT NULL DEFAULT '{}',
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS user_machines (
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  machine_id TEXT NOT NULL REFERENCES machines(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'owner',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, machine_id)
);

CREATE INDEX IF NOT EXISTS idx_user_machines_machine_id ON user_machines(machine_id);

CREATE TABLE IF NOT EXISTS machine_states (
  machine_id TEXT PRIMARY KEY REFERENCES machines(id) ON DELETE CASCADE,
  status TEXT NOT NULL,
  desired_status TEXT NOT NULL,
  container_id TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  updated_at BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS machine_jobs (
  id TEXT PRIMARY KEY,
  machine_id TEXT NOT NULL REFERENCES machines(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  status TEXT NOT NULL,
  attempt INTEGER NOT NULL DEFAULT 0,
  next_run_at BIGINT NOT NULL,
  lease_owner TEXT,
  lease_until BIGINT,
  last_error TEXT,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_machine_jobs_status_next_run_at
  ON machine_jobs(status, next_run_at);

CREATE INDEX IF NOT EXISTS idx_machine_jobs_lease_until
  ON machine_jobs(status, lease_until);

CREATE TABLE IF NOT EXISTS machine_events (
  id TEXT PRIMARY KEY,
  machine_id TEXT NOT NULL,
  job_id TEXT NOT NULL DEFAULT '',
  level TEXT NOT NULL DEFAULT 'info',
  event_type TEXT NOT NULL,
  message TEXT NOT NULL,
  created_at BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_machine_events_machine_id_created_at
  ON machine_events(machine_id, created_at DESC);

CREATE TABLE IF NOT EXISTS setup_state (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  completed BOOLEAN NOT NULL DEFAULT FALSE,
  base_domain TEXT NOT NULL DEFAULT '',
  domain_prefix TEXT NOT NULL DEFAULT '',
  cloudflare_api_token TEXT NOT NULL DEFAULT '',
  updated_at BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS machine_tokens (
  id TEXT PRIMARY KEY,
  machine_id TEXT NOT NULL UNIQUE REFERENCES machines(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  created_at BIGINT NOT NULL,
  revoked_at BIGINT
);

CREATE INDEX IF NOT EXISTS idx_machine_tokens_token_hash ON machine_tokens(token_hash);

CREATE TABLE IF NOT EXISTS auth_tickets (
  id TEXT PRIMARY KEY,
  ticket_hash TEXT NOT NULL UNIQUE,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  machine_id TEXT NOT NULL REFERENCES machines(id) ON DELETE CASCADE,
  exposure_id TEXT NOT NULL DEFAULT '',
  expires_at BIGINT NOT NULL,
  used_at BIGINT,
  created_at BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_auth_tickets_machine_id ON auth_tickets(machine_id);
CREATE INDEX IF NOT EXISTS idx_auth_tickets_expires_at ON auth_tickets(expires_at);

CREATE TABLE IF NOT EXISTS machine_tunnels (
  machine_id TEXT PRIMARY KEY REFERENCES machines(id) ON DELETE CASCADE,
  account_id TEXT NOT NULL,
  tunnel_id TEXT NOT NULL UNIQUE,
  tunnel_name TEXT NOT NULL,
  tunnel_token TEXT NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS machine_exposures (
  id TEXT PRIMARY KEY,
  machine_id TEXT NOT NULL REFERENCES machines(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  hostname TEXT NOT NULL UNIQUE,
  service TEXT NOT NULL,
  is_public BOOLEAN NOT NULL DEFAULT FALSE,
  visibility TEXT NOT NULL DEFAULT 'owner_only',
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  UNIQUE(machine_id, name)
);

CREATE INDEX IF NOT EXISTS idx_machine_exposures_machine_id ON machine_exposures(machine_id);

CREATE TABLE IF NOT EXISTS machine_exposure_acl_users (
  exposure_id TEXT NOT NULL REFERENCES machine_exposures(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at BIGINT NOT NULL,
  PRIMARY KEY (exposure_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_machine_exposure_acl_users_user_id ON machine_exposure_acl_users(user_id);
