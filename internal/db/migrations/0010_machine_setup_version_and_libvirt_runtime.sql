ALTER TABLE machines
ADD COLUMN setup_version TEXT NOT NULL DEFAULT '';

UPDATE machines
SET runtime = 'libvirt'
WHERE LOWER(TRIM(runtime)) = 'docker';

INSERT INTO app_meta (key, value)
VALUES ('setup.machine_runtime', 'libvirt')
ON CONFLICT (key) DO UPDATE
SET value = excluded.value
WHERE LOWER(TRIM(app_meta.value)) = 'docker' OR TRIM(app_meta.value) = '';

UPDATE setup_state
SET docker_provider_enabled = FALSE
WHERE docker_provider_enabled = TRUE;
