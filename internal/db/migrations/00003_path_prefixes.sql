-- +goose Up
ALTER TABLE crawls ADD COLUMN path_prefixes TEXT[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE crawls DROP COLUMN path_prefixes;
