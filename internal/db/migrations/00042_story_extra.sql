-- +goose Up
-- Story extras written with the why-it-matters line: what led to the story, who it affects, and what to watch
-- next with a date when the article gives one. A small JSON object, validated by the API before it is stored.
ALTER TABLE pages ADD COLUMN extra JSONB;
-- The Week ahead page reads the next-event date, so it gets an index of its own, only over rows that have one.
CREATE INDEX pages_next_date_idx ON pages ((extra->'next'->>'date')) WHERE extra->'next'->>'date' IS NOT NULL;

-- The events table was made for upcoming dates found with rules in article text. Nothing reads or writes it.
DROP TABLE IF EXISTS events;

-- +goose Down
CREATE TABLE events (
    day        DATE        NOT NULL,
    story_id   TEXT        NOT NULL,
    text_hash  TEXT        NOT NULL,
    text       TEXT        NOT NULL,
    title      TEXT        NOT NULL,
    source     TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (day, story_id, text_hash)
);
DROP INDEX pages_next_date_idx;
ALTER TABLE pages DROP COLUMN extra;
