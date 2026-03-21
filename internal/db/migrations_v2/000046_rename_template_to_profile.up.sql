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

-- Recreate machines table with renamed columns, FK to machine_profiles, and new column.
-- SQLite does not support adding FK constraints via ALTER TABLE,
-- so we use the create-copy-drop-rename pattern.
CREATE TABLE machines_new (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  profile_id TEXT NOT NULL DEFAULT 'libvirt' REFERENCES machine_profiles(id) ON DELETE RESTRICT,
  provider_type TEXT NOT NULL DEFAULT '',
  infrastructure_config_json TEXT NOT NULL DEFAULT '{}',
  applied_boot_config_hash TEXT NOT NULL DEFAULT '',
  setup_version TEXT NOT NULL DEFAULT '',
  options_json TEXT NOT NULL DEFAULT '{}',
  custom_image_id TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO machines_new (id, name, profile_id, provider_type, infrastructure_config_json, setup_version, options_json, custom_image_id, created_at)
SELECT id, name, template_id, template_type,
  json_remove(
    json_remove(
      json_remove(
        json_remove(
          json_remove(
            json_remove(
              json_remove(
                json_remove(
                  json_remove(
                    json_remove(template_config_json,
                      '$.serverApiUrl'),
                    '$.server_api_url'),
                  '$.autoStopTimeoutSeconds'),
                '$.auto_stop_timeout_seconds'),
              '$.libvirt.startup_script'),
            '$.libvirt.startupScript'),
          '$.gce.startup_script'),
        '$.gce.startupScript'),
      '$.lxd.startup_script'),
    '$.lxd.startupScript'),
  setup_version, options_json, custom_image_id, created_at
FROM machines;

DROP TABLE machines;
ALTER TABLE machines_new RENAME TO machines;
