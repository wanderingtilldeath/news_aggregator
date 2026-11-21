-- +goose Up
-- How often a subscriber gets the edition: every morning (daily) or once a week, on the weekly day (weekly).
ALTER TABLE subscribers ADD COLUMN frequency TEXT NOT NULL DEFAULT 'daily' CHECK (frequency IN ('daily', 'weekly'));

-- +goose Down
ALTER TABLE subscribers DROP COLUMN frequency;
