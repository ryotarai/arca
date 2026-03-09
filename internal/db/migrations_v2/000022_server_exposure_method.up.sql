-- Add server exposure method and server domain to app_meta.
-- Existing installations default to cloudflare_tunnel with a derived server domain.

-- Populate server_exposure_method for existing completed setups.
INSERT INTO app_meta (key, value)
SELECT 'setup.server_exposure_method', 'cloudflare_tunnel'
WHERE EXISTS (SELECT 1 FROM setup_state WHERE completed = TRUE)
  AND NOT EXISTS (SELECT 1 FROM app_meta WHERE key = 'setup.server_exposure_method');

-- Derive and populate server_domain from existing domain_prefix + base_domain.
-- Format: {prefix}app.{base_domain}
INSERT INTO app_meta (key, value)
SELECT 'setup.server_domain',
       CASE
         WHEN s.domain_prefix = '' THEN 'app.' || s.base_domain
         ELSE RTRIM(s.domain_prefix || 'app', '-') || '.' || s.base_domain
       END
FROM setup_state s
WHERE s.completed = TRUE
  AND s.base_domain != ''
  AND NOT EXISTS (SELECT 1 FROM app_meta WHERE key = 'setup.server_domain');
