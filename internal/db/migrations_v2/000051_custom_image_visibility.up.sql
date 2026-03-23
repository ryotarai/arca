ALTER TABLE custom_images ADD COLUMN created_by_user_id TEXT NOT NULL DEFAULT '';
ALTER TABLE custom_images ADD COLUMN visibility TEXT NOT NULL DEFAULT 'private';

-- Existing images become shared so they remain accessible after migration
UPDATE custom_images SET visibility = 'shared' WHERE visibility = 'private';

CREATE INDEX IF NOT EXISTS idx_custom_images_visibility_user ON custom_images(visibility, created_by_user_id);
