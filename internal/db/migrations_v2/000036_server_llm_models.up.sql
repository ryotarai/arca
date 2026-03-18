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
