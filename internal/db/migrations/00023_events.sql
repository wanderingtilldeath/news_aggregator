-- +goose Up
-- Upcoming events found in article text ("the committee meets on 4 December"): the day, the sentence,
-- and the story it came from. text_hash makes a repeated crawl of a story add each event once.
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

-- +goose Down
DROP TABLE events;
