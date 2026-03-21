CREATE TABLE IF NOT EXISTS machine_tags (
    machine_id TEXT NOT NULL REFERENCES machines(id) ON DELETE CASCADE,
    tag TEXT NOT NULL,
    PRIMARY KEY (machine_id, tag)
);
CREATE INDEX IF NOT EXISTS idx_machine_tags_tag ON machine_tags(tag);
