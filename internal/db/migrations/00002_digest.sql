-- +goose Up
-- published_at comes from page metadata; first_seen_at is set once and never
-- rewritten, so undated pages still sort by when we first found them.
ALTER TABLE pages
    ADD COLUMN published_at  TIMESTAMPTZ,
    ADD COLUMN first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE INDEX pages_recent_idx ON pages ((COALESCE(published_at, first_seen_at)) DESC);

-- +goose Down
DROP INDEX pages_recent_idx;
ALTER TABLE pages DROP COLUMN first_seen_at, DROP COLUMN published_at;
