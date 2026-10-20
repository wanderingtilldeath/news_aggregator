-- +goose Up
-- image_url, is_article and content are filled by the crawler; summary and
-- summarized_at by the daily summarise pass (content is its input).
ALTER TABLE pages
    ADD COLUMN image_url     TEXT        NOT NULL DEFAULT '',
    ADD COLUMN is_article    BOOLEAN     NOT NULL DEFAULT false,
    ADD COLUMN content       TEXT        NOT NULL DEFAULT '',
    ADD COLUMN summary       TEXT[]      NOT NULL DEFAULT '{}',
    ADD COLUMN summarized_at TIMESTAMPTZ;
CREATE INDEX pages_unsummarized_idx ON pages (first_seen_at) WHERE is_article AND summarized_at IS NULL;

-- One row per scheduled job per day, so several crawler processes run it once.
CREATE TABLE job_runs (
    job        TEXT        NOT NULL,
    run_day    DATE        NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (job, run_day)
);

-- +goose Down
DROP TABLE job_runs;
DROP INDEX pages_unsummarized_idx;
ALTER TABLE pages
    DROP COLUMN summarized_at, DROP COLUMN summary, DROP COLUMN content,
    DROP COLUMN is_article, DROP COLUMN image_url;
