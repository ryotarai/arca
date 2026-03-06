ALTER TABLE machine_exposures ADD COLUMN visibility TEXT NOT NULL DEFAULT 'owner_only';

UPDATE machine_exposures
SET visibility = CASE
  WHEN is_public THEN 'internet_public'
  ELSE 'owner_only'
END
WHERE visibility = '' OR visibility IS NULL;

CREATE TABLE IF NOT EXISTS machine_exposure_acl_users (
  exposure_id TEXT NOT NULL REFERENCES machine_exposures(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at BIGINT NOT NULL,
  PRIMARY KEY (exposure_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_machine_exposure_acl_users_user_id ON machine_exposure_acl_users(user_id);
