-- +goose Up
-- The short "Today in 60 seconds" briefing, one per edition day and track, written from the day's
-- top headlines and their why-it-matters lines (no article text, so it costs few tokens).
CREATE TABLE intros (
    edition_day DATE        NOT NULL,
    track       TEXT        NOT NULL,
    body        TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (edition_day, track)
);

-- +goose Down
DROP TABLE intros;
