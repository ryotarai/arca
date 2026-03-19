-- Rename runtimes table to machine_templates
ALTER TABLE runtimes RENAME TO machine_templates;

-- Rename machines columns: runtime_id -> template_id, runtime_type -> template_type, runtime_config_json -> template_config_json
ALTER TABLE machines RENAME COLUMN runtime_id TO template_id;
ALTER TABLE machines RENAME COLUMN runtime_type TO template_type;
ALTER TABLE machines RENAME COLUMN runtime_config_json TO template_config_json;

-- Rename runtime_custom_images table and columns
ALTER TABLE runtime_custom_images RENAME TO template_custom_images;
ALTER TABLE template_custom_images RENAME COLUMN runtime_id TO template_id;

-- Rename custom_images column: runtime_type -> template_type
ALTER TABLE custom_images RENAME COLUMN runtime_type TO template_type;
