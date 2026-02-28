-- Keep only the newest machine per duplicated name, then enforce uniqueness.
WITH ranked AS (
  SELECT
    id,
    ROW_NUMBER() OVER (PARTITION BY name ORDER BY created_at DESC, id DESC) AS rn
  FROM machines
)
DELETE FROM machines
WHERE id IN (
  SELECT id
  FROM ranked
  WHERE rn > 1
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_machines_name_unique ON machines(name);
