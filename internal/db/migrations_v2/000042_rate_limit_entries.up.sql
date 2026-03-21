CREATE TABLE IF NOT EXISTS rate_limit_entries (
    key TEXT NOT NULL,
    timestamp_unix BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_rate_limit_entries_key_ts ON rate_limit_entries(key, timestamp_unix);
