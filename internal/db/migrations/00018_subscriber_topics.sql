-- +goose Up
-- Words a subscriber follows (up to 5); the email adds a "Your topics" section for them.
ALTER TABLE subscribers ADD COLUMN topics TEXT[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE subscribers DROP COLUMN topics;
