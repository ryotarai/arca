ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'user';

UPDATE users SET role = 'admin'
WHERE id = (
  SELECT admin_user_id FROM setup_state WHERE id = 1 AND admin_user_id IS NOT NULL
);
