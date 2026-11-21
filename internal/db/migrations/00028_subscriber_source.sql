-- +goose Up
-- Where a signup came from: the campaign tag of the link (?utm_source=). Empty is a direct visit. The
-- analytics page counts signups and confirmations per source.
ALTER TABLE subscribers ADD COLUMN source TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE subscribers DROP COLUMN source;
