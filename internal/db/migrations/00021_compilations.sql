-- +goose Up
-- The monthly current-affairs compilation: for each month and edition, the most important stories
-- grouped by syllabus area, as JSON. It is built a little each day from that day's top stories, so
-- it outlives the pages themselves (those are deleted after 60 days).
CREATE TABLE compilations (
    month    TEXT        NOT NULL,
    track    TEXT        NOT NULL,
    areas    JSONB       NOT NULL,
    built_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (month, track)
);

-- +goose Down
DROP TABLE compilations;
