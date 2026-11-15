-- +goose Up
-- The long summary and exam lens are now written in a second, selective stage (only for the most
-- important stories); detailed_at marks it done. Stories that already have either are marked done.
ALTER TABLE pages ADD COLUMN detailed_at TIMESTAMPTZ;
UPDATE pages SET detailed_at = COALESCE(enriched_at, now()) WHERE long_summary <> '' OR exam IS NOT NULL;

-- +goose Down
ALTER TABLE pages DROP COLUMN detailed_at;
