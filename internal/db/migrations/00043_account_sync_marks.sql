-- +goose Up
-- The places a reader starred on the revision maps and the notes they wrote there, carried between devices with the rest of
-- the free account's synced items. A JSON array, validated and size-capped by the API.
ALTER TABLE account_sync ADD COLUMN marks JSONB NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE account_sync DROP COLUMN marks;
