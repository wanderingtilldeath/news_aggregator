-- +goose Up
-- body_image_url is the first real picture inside the article: the fallback when
-- the page's og:image (image_url) is missing or a logo shared by many articles.
ALTER TABLE pages ADD COLUMN body_image_url TEXT NOT NULL DEFAULT '';
CREATE INDEX pages_image_url_idx ON pages (image_url) WHERE is_article AND image_url <> '';
CREATE INDEX pages_body_image_url_idx ON pages (body_image_url) WHERE is_article AND body_image_url <> '';

-- +goose Down
DROP INDEX pages_body_image_url_idx;
DROP INDEX pages_image_url_idx;
ALTER TABLE pages DROP COLUMN body_image_url;
