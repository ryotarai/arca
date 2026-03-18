DROP TABLE IF EXISTS machine_tunnels;
DELETE FROM app_meta WHERE key IN ('setup.cloudflare_zone_id', 'setup.server_exposure_method');
