-- +goose Up
-- An optional email on an account, so a reader can log in with it instead of the username. It is not verified and
-- nothing is mailed to it; NULL for the many accounts that have none.
ALTER TABLE accounts ADD COLUMN email TEXT CHECK (email = lower(email));
CREATE UNIQUE INDEX accounts_email_key ON accounts (email);

-- +goose Down
DROP INDEX accounts_email_key;
ALTER TABLE accounts DROP COLUMN email;
