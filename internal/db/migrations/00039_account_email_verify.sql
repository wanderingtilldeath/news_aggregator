-- +goose Up
-- An account's email only counts once the reader has opened the link mailed to it. Until then it can neither log in
-- nor receive a password reset, and it does not block anyone else from claiming the address (so a stranger cannot
-- squat on it). Existing emails start unverified; their owners confirm from the account page.
ALTER TABLE accounts ADD COLUMN email_verified_at TIMESTAMPTZ;
DROP INDEX accounts_email_key;
CREATE UNIQUE INDEX accounts_email_key ON accounts (email) WHERE email_verified_at IS NOT NULL;

-- One-time links mailed to an address: "verify" confirms it, "reset" sets a new password. Only the hash of the secret
-- is stored. The email is kept so a link made for an address stops working after the address changes.
CREATE TABLE account_email_tokens (
    token_hash TEXT        PRIMARY KEY,
    account_id BIGINT      NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    purpose    TEXT        NOT NULL CHECK (purpose IN ('verify', 'reset')),
    email      TEXT        NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX account_email_tokens_account_idx ON account_email_tokens (account_id);

-- +goose Down
DROP TABLE account_email_tokens;
DROP INDEX accounts_email_key;
CREATE UNIQUE INDEX accounts_email_key ON accounts (email);
ALTER TABLE accounts DROP COLUMN email_verified_at;
