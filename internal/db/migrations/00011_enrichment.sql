-- +goose Up
-- Extra reader-facing fields written by the enrichment pass (an LLM): a "why it
-- matters" line, topic tags, an importance score from 1 (minor) to 5 (major), and
-- a longer summary for the 7-minute reading mode. enriched_at marks the pass done.
ALTER TABLE pages
    ADD COLUMN why          TEXT        NOT NULL DEFAULT '',
    ADD COLUMN tags         TEXT[]      NOT NULL DEFAULT '{}',
    ADD COLUMN importance   SMALLINT    NOT NULL DEFAULT 0,
    ADD COLUMN long_summary TEXT        NOT NULL DEFAULT '',
    ADD COLUMN enriched_at  TIMESTAMPTZ;

-- Summarised articles still waiting for enrichment, newest first.
CREATE INDEX pages_pending_enrichment_idx ON pages ((COALESCE(published_at, first_seen_at)) DESC)
    WHERE is_article AND summarized_at IS NOT NULL AND enriched_at IS NULL;

-- +goose Down
DROP INDEX pages_pending_enrichment_idx;
ALTER TABLE pages
    DROP COLUMN enriched_at, DROP COLUMN long_summary, DROP COLUMN importance,
    DROP COLUMN tags, DROP COLUMN why;
