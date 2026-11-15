-- +goose Up
-- Slack and Discord channels that get the daily edition: the incoming-webhook address (a secret, so it
-- is never shown again), the edition and a token for the "stop these posts" link.
CREATE TABLE team_hooks (
    id         BIGSERIAL   PRIMARY KEY,
    kind       TEXT        NOT NULL,
    url        TEXT        NOT NULL UNIQUE,
    track      TEXT        NOT NULL,
    token      TEXT        NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE team_hooks;
