-- +goose Up
-- One generated quiz per edition day and track: a JSON array of questions, each
-- {question, options[4], answer (index), explanation, url}.
CREATE TABLE quizzes (
    edition_day DATE        NOT NULL,
    track       TEXT        NOT NULL,
    questions   JSONB       NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (edition_day, track)
);

-- +goose Down
DROP TABLE quizzes;
