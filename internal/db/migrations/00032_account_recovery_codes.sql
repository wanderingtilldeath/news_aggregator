-- +goose Up
-- Ten recovery codes instead of one, and sign-in with a provider. The single recovery code of 00031 becomes the first
-- row of account_recovery_codes before its column is dropped. An account that signs in with Google alone has no
-- password, so password_hash may be empty. An identity is a provider's own subject id, never an email.
CREATE TABLE account_recovery_codes (
    account_id BIGINT      NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    code_hash  TEXT        NOT NULL,
    used_at    TIMESTAMPTZ,
    PRIMARY KEY (account_id, code_hash)
);
INSERT INTO account_recovery_codes (account_id, code_hash) SELECT id, recovery_hash FROM accounts;
ALTER TABLE accounts DROP COLUMN recovery_hash;
ALTER TABLE accounts ALTER COLUMN password_hash SET DEFAULT '';

CREATE TABLE account_identities (
    provider   TEXT   NOT NULL,
    subject    TEXT   NOT NULL,
    account_id BIGINT NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    PRIMARY KEY (provider, subject)
);
CREATE INDEX account_identities_account_idx ON account_identities (account_id);

-- +goose Down
DROP TABLE account_identities;
ALTER TABLE accounts ADD COLUMN recovery_hash TEXT NOT NULL DEFAULT '';
UPDATE accounts SET recovery_hash = COALESCE((SELECT code_hash FROM account_recovery_codes c WHERE c.account_id = accounts.id ORDER BY used_at NULLS FIRST LIMIT 1), '');
DROP TABLE account_recovery_codes;
