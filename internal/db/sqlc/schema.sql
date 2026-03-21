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

CREATE TABLE IF NOT EXISTS user_settings (
  user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  ssh_public_keys_json TEXT NOT NULL DEFAULT '[]',
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL
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
  template_id TEXT NOT NULL DEFAULT 'libvirt',
  template_type TEXT NOT NULL DEFAULT '',
  template_config_json TEXT NOT NULL DEFAULT '{}',
  setup_version TEXT NOT NULL DEFAULT '',
  options_json TEXT NOT NULL DEFAULT '{}',
  custom_image_id TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS machine_templates (
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
  role TEXT NOT NULL DEFAULT 'admin',
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
  ready BOOLEAN NOT NULL DEFAULT FALSE,
  ready_reported_at BIGINT NOT NULL DEFAULT 0,
  ready_reason TEXT NOT NULL DEFAULT '',
  updated_at BIGINT NOT NULL,
  last_activity_at BIGINT NOT NULL DEFAULT 0,
  arcad_version TEXT NOT NULL DEFAULT ''
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

CREATE INDEX IF NOT EXISTS idx_machine_jobs_machine_id_status
  ON machine_jobs(machine_id, status);

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
  updated_at BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS machine_tokens (
  id TEXT PRIMARY KEY,
  machine_id TEXT NOT NULL UNIQUE REFERENCES machines(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  token TEXT NOT NULL DEFAULT '',
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

CREATE TABLE IF NOT EXISTS machine_sharing (
  machine_id TEXT PRIMARY KEY REFERENCES machines(id) ON DELETE CASCADE,
  general_access_scope TEXT NOT NULL DEFAULT 'none',
  general_access_role TEXT NOT NULL DEFAULT 'none',
  updated_at BIGINT NOT NULL DEFAULT 0
);

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

CREATE TABLE IF NOT EXISTS user_notification_settings (
  user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  slack_enabled BOOLEAN NOT NULL DEFAULT true,
  slack_user_id TEXT NOT NULL DEFAULT '',
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL
);

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

CREATE TABLE IF NOT EXISTS user_llm_models (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  config_name TEXT NOT NULL,
  endpoint_type TEXT NOT NULL,
  custom_endpoint TEXT NOT NULL DEFAULT '',
  model_name TEXT NOT NULL,
  api_key_encrypted TEXT NOT NULL,
  max_context_tokens INTEGER NOT NULL DEFAULT 0,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  UNIQUE(user_id, config_name)
);
CREATE INDEX IF NOT EXISTS idx_user_llm_models_user_id ON user_llm_models(user_id);

CREATE TABLE IF NOT EXISTS custom_images (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  template_type TEXT NOT NULL,
  data_json TEXT NOT NULL DEFAULT '{}',
  description TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(name, template_type)
);

CREATE TABLE IF NOT EXISTS template_custom_images (
  template_id TEXT NOT NULL REFERENCES machine_templates(id) ON DELETE CASCADE,
  custom_image_id TEXT NOT NULL REFERENCES custom_images(id) ON DELETE CASCADE,
  PRIMARY KEY (template_id, custom_image_id)
);
CREATE INDEX IF NOT EXISTS idx_template_custom_images_image ON template_custom_images(custom_image_id);

CREATE TABLE IF NOT EXISTS audit_logs (
  id TEXT PRIMARY KEY,
  actor_user_id TEXT NOT NULL,
  acting_as_user_id TEXT DEFAULT NULL,
  action TEXT NOT NULL,
  resource_type TEXT NOT NULL DEFAULT '',
  resource_id TEXT NOT NULL DEFAULT '',
  details_json TEXT NOT NULL DEFAULT '{}',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor ON audit_logs(actor_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_acting_as ON audit_logs(acting_as_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at DESC);

CREATE TABLE IF NOT EXISTS server_llm_models (
  id TEXT PRIMARY KEY,
  config_name TEXT NOT NULL UNIQUE,
  endpoint_type TEXT NOT NULL,
  custom_endpoint TEXT NOT NULL DEFAULT '',
  model_name TEXT NOT NULL,
  token_command TEXT NOT NULL,
  max_context_tokens INTEGER NOT NULL DEFAULT 0,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS admin_view_mode (
  user_id TEXT PRIMARY KEY NOT NULL,
  mode TEXT NOT NULL DEFAULT 'admin',
  updated_at BIGINT NOT NULL
);
