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
