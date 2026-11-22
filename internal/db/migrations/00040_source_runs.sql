-- +goose Up
-- One row per source per daily run: how many pages the crawl fetched, how many of them are articles, how many failed, and
-- how it ended. The crawls table keeps only three days and not the source's name, so this is what shows a source that
-- quietly stopped working. Rows older than 60 days are deleted when a run records.
CREATE TABLE source_runs (
    id       BIGSERIAL   PRIMARY KEY,
    at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    source   TEXT        NOT NULL,
    host     TEXT        NOT NULL,
    pages    INTEGER     NOT NULL,
    articles INTEGER     NOT NULL,
    failed   INTEGER     NOT NULL,
    outcome  TEXT        NOT NULL CHECK (outcome IN ('ok', 'empty', 'failed', 'timeout'))
);
CREATE INDEX source_runs_source_at_idx ON source_runs (source, at DESC);

-- +goose Down
DROP TABLE source_runs;
