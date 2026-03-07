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
