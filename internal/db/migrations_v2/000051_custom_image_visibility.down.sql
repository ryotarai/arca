DROP INDEX IF EXISTS idx_custom_images_visibility_user;
ALTER TABLE custom_images DROP COLUMN visibility;
ALTER TABLE custom_images DROP COLUMN created_by_user_id;
