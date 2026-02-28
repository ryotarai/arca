CREATE TABLE IF NOT EXISTS setup_state (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  completed BOOLEAN NOT NULL DEFAULT FALSE,
  admin_user_id TEXT,
  base_domain TEXT NOT NULL DEFAULT '',
  cloudflare_api_token TEXT NOT NULL DEFAULT '',
  docker_provider_enabled BOOLEAN NOT NULL DEFAULT FALSE,
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
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  UNIQUE(machine_id, name)
);

CREATE INDEX IF NOT EXISTS idx_machine_exposures_machine_id ON machine_exposures(machine_id);
