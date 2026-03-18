CREATE TABLE IF NOT EXISTS custom_images (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  runtime_type TEXT NOT NULL,
  data_json TEXT NOT NULL DEFAULT '{}',
  description TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(name, runtime_type)
);

CREATE TABLE IF NOT EXISTS runtime_custom_images (
  runtime_id TEXT NOT NULL REFERENCES runtimes(id) ON DELETE CASCADE,
  custom_image_id TEXT NOT NULL REFERENCES custom_images(id) ON DELETE CASCADE,
  PRIMARY KEY (runtime_id, custom_image_id)
);
CREATE INDEX IF NOT EXISTS idx_runtime_custom_images_image ON runtime_custom_images(custom_image_id);

ALTER TABLE machines ADD COLUMN custom_image_id TEXT NOT NULL DEFAULT '';
