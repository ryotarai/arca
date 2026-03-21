DROP INDEX IF EXISTS idx_machine_exposures_machine_id;
DROP TABLE IF EXISTS machine_exposures;
ALTER TABLE machines DROP COLUMN IF EXISTS endpoint;
