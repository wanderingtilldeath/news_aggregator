-- +goose Up
-- Admin sign-in: a one-time sign-in link ("login") and the session it opens ("session"). Only the SHA-256 of the
-- secret is stored, so a copy of the table opens nothing. Rows are short-lived and removed once expired.
CREATE TABLE admin_tokens (
    token_hash TEXT        PRIMARY KEY,
    kind       TEXT        NOT NULL CHECK (kind IN ('login', 'session')),
    email      TEXT        NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX admin_tokens_expires_idx ON admin_tokens (expires_at);

-- +goose Down
DROP TABLE admin_tokens;
