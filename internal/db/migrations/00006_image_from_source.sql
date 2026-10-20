-- +goose Up
-- image_from_source: after crawling, fetch each article's cited source page and
-- use its og:image (for sites whose own pages have only a generic logo).
-- source_url is that cited page; image_checked_at marks that it was looked up.
ALTER TABLE crawls ADD COLUMN image_from_source BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE pages
    ADD COLUMN source_url       TEXT        NOT NULL DEFAULT '',
    ADD COLUMN image_checked_at TIMESTAMPTZ;
CREATE INDEX pages_unchecked_images_idx ON pages (first_seen_at)
    WHERE is_article AND source_url <> '' AND image_checked_at IS NULL;

-- +goose Down
DROP INDEX pages_unchecked_images_idx;
ALTER TABLE pages DROP COLUMN image_checked_at, DROP COLUMN source_url;
ALTER TABLE crawls DROP COLUMN image_from_source;
