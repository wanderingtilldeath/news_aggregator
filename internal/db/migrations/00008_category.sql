-- +goose Up
-- category groups a source's articles in the digest (tech, ai, upsc, ...).
-- '' means the source set none; the digest shows it as "other".
ALTER TABLE crawls ADD COLUMN category TEXT NOT NULL DEFAULT '';
ALTER TABLE pages ADD COLUMN category TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE pages DROP COLUMN category;
ALTER TABLE crawls DROP COLUMN category;
