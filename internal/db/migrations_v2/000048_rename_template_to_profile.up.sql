-- Rename machine_templates table to machine_profiles and add boot_config_hash
ALTER TABLE machine_templates RENAME TO machine_profiles;
ALTER TABLE machine_profiles ADD COLUMN boot_config_hash TEXT NOT NULL DEFAULT '';
UPDATE machine_profiles SET boot_config_hash = 'migrated-' || id;

-- Rename template_custom_images table to profile_custom_images
ALTER TABLE template_custom_images RENAME TO profile_custom_images;
ALTER TABLE profile_custom_images RENAME COLUMN template_id TO profile_id;

-- Rename the index to match the new table name (SQLite preserves old index name on table rename)
DROP INDEX IF EXISTS idx_template_custom_images_image;
CREATE INDEX IF NOT EXISTS idx_profile_custom_images_image ON profile_custom_images(custom_image_id);

-- Rename custom_images column: template_type -> provider_type
ALTER TABLE custom_images RENAME COLUMN template_type TO provider_type;

-- Rename machines columns instead of recreating the table.
-- ALTER TABLE RENAME COLUMN works in both SQLite (3.25+) and PostgreSQL,
-- and avoids issues with json_remove (SQLite-only) and DROP TABLE FK conflicts.
ALTER TABLE machines RENAME COLUMN template_id TO profile_id;
ALTER TABLE machines RENAME COLUMN template_type TO provider_type;
ALTER TABLE machines RENAME COLUMN template_config_json TO infrastructure_config_json;
ALTER TABLE machines ADD COLUMN applied_boot_config_hash TEXT NOT NULL DEFAULT '';
