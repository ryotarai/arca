ALTER TABLE machines
  ADD COLUMN options_json TEXT NOT NULL DEFAULT '{}';
