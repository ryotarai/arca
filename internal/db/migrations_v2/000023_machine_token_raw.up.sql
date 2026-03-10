-- Store the raw machine token so it can be passed to cloud-init for arcad download.
ALTER TABLE machine_tokens ADD COLUMN token TEXT NOT NULL DEFAULT '';
