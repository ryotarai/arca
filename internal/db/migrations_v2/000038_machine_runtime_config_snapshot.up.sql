ALTER TABLE machines ADD COLUMN runtime_type TEXT NOT NULL DEFAULT '';
ALTER TABLE machines ADD COLUMN runtime_config_json TEXT NOT NULL DEFAULT '{}';

-- Backfill existing machines from their runtime catalog entries.
UPDATE machines SET
  runtime_type = COALESCE((SELECT type FROM runtimes WHERE runtimes.id = machines.runtime_id), ''),
  runtime_config_json = COALESCE((SELECT config_json FROM runtimes WHERE runtimes.id = machines.runtime_id), '{}')
WHERE runtime_type = '' AND runtime_id != '';
