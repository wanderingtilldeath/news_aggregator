-- +goose Up
-- What a free account carries between devices: bookmarks, flashcard progress, the reading streak and the quiz
-- questions missed. One row per account, one value per kind, validated and size-capped by the API. The notebook is a
-- Pro feature and has its own table.
CREATE TABLE account_sync (
    account_id BIGINT      PRIMARY KEY REFERENCES accounts (id) ON DELETE CASCADE,
    bookmarks  JSONB       NOT NULL DEFAULT '[]',
    cards      JSONB       NOT NULL DEFAULT '{}',
    streak     TEXT        NOT NULL DEFAULT '',
    missed     JSONB       NOT NULL DEFAULT '[]',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE account_sync;
