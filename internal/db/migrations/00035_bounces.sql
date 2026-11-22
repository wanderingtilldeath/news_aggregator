-- +goose Up
-- Set by the Brevo webhook when an address hard-bounces, is blocked or reports spam. The daily send skips these
-- addresses, so a dead one is not tried every run and a complaint is not mailed again.
ALTER TABLE subscribers ADD COLUMN bounced_at TIMESTAMPTZ;
ALTER TABLE subscribers ADD COLUMN bounce_reason TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE subscribers DROP COLUMN bounce_reason;
ALTER TABLE subscribers DROP COLUMN bounced_at;
