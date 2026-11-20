-- +goose Up
-- The length of the article in words, counted before its text is cut for summaries, so a card can say
-- "4 min read". Existing rows get 0 (unknown) until their next crawl.
ALTER TABLE pages ADD COLUMN words INT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE pages DROP COLUMN words;
