-- +goose Up
-- Email subscribers for the daily digest. A row is created unconfirmed; the
-- reader confirms by using the token from the email (double opt-in).
-- The token is random, single per subscriber, and also serves as the
-- unsubscribe key.
CREATE TABLE subscribers (
    id              BIGSERIAL PRIMARY KEY,
    email           TEXT        NOT NULL CHECK (email = lower(email)),
    token           TEXT        NOT NULL,
    confirmed_at    TIMESTAMPTZ,
    unsubscribed_at TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX subscribers_email_key ON subscribers (email);
CREATE UNIQUE INDEX subscribers_token_key ON subscribers (token);

-- +goose Down
DROP TABLE subscribers;
