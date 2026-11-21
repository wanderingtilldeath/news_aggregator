-- +goose Up
-- The Pro notebook: the sentences a Pro member saved from stories, with their own note and tags, kept on the
-- server so they follow the member to another device. at is when it was saved, in milliseconds, as the browser
-- counts it; updated_at is the server's last change. Notes go with the member if the member row is deleted.
CREATE TABLE pro_notes (
    email      TEXT        NOT NULL REFERENCES pro_members (email) ON DELETE CASCADE,
    id         TEXT        NOT NULL,
    story      TEXT        NOT NULL,
    title      TEXT        NOT NULL,
    text       TEXT        NOT NULL,
    note       TEXT        NOT NULL DEFAULT '',
    tags       TEXT[]      NOT NULL DEFAULT '{}',
    at         BIGINT      NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (email, id)
);
CREATE INDEX pro_notes_recent_idx ON pro_notes (email, at DESC);

-- +goose Down
DROP TABLE pro_notes;
