-- +goose Up
-- One row per channel per edition day once the day's edition was posted there (for
-- example to a Telegram channel), so running the daily send again posts nothing twice.
CREATE TABLE channel_posts (
    edition_day DATE        NOT NULL,
    channel     TEXT        NOT NULL,
    posted_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (edition_day, channel)
);

-- +goose Down
DROP TABLE channel_posts;
