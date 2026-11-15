-- +goose Up
-- Plain-language explainers for the concepts behind the news ("repo rate", "writ"). The detail stage
-- names the terms a story depends on; a term waits with an empty body until one model call writes
-- the explainer, once, for every story that mentions it.
CREATE TABLE concepts (
    term_key   TEXT        PRIMARY KEY,
    term       TEXT        NOT NULL,
    body       TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE concepts;
