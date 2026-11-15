-- +goose Up
ALTER TABLE pages ADD COLUMN story_id TEXT;
ALTER TABLE pages ADD COLUMN coverage INT NOT NULL DEFAULT 0;
CREATE INDEX pages_story_id_idx ON pages (story_id) WHERE story_id IS NOT NULL;

-- +goose Down
DROP INDEX pages_story_id_idx;
ALTER TABLE pages DROP COLUMN coverage;
ALTER TABLE pages DROP COLUMN story_id;
