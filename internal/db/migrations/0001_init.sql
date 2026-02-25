CREATE TABLE IF NOT EXISTS app_meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

INSERT INTO app_meta (key, value)
VALUES ('schema_version', '1')
ON CONFLICT (key) DO UPDATE
SET value = excluded.value;
