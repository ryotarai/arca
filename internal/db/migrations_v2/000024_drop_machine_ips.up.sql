-- Machine IPs (private_ip, public_ip) are no longer stored in the DB.
-- They are resolved on-demand via an in-memory cache backed by the runtime.
-- The columns may still exist in the table from a previous migration but
-- are no longer read or written by the application.
-- SQLite cannot reliably DROP COLUMN when the table has foreign key references,
-- so we leave the columns in place as harmless no-ops.
SELECT 1;
