-- +goose Up
-- Reader accounts, with no email: a username and a password (stored only as a salted PBKDF2 hash) and a recovery code
-- (stored only as its SHA-256), which is shown once at signup and resets the password. pro_email links the account to
-- a Pro membership; one membership belongs to one account. Sessions are stored by the hash of their secret.
CREATE TABLE accounts (
    id            BIGSERIAL   PRIMARY KEY,
    username      TEXT        NOT NULL CHECK (username = lower(username)),
    password_hash TEXT        NOT NULL,
    recovery_hash TEXT        NOT NULL,
    pro_email     TEXT        UNIQUE REFERENCES pro_members (email) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX accounts_username_key ON accounts (username);

CREATE TABLE account_sessions (
    token_hash TEXT        PRIMARY KEY,
    account_id BIGINT      NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX account_sessions_account_idx ON account_sessions (account_id);

-- +goose Down
DROP TABLE account_sessions;
DROP TABLE accounts;
