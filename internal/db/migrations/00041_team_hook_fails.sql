-- +goose Up
-- How many posts in a row the service refused (400, 401 or 403). A channel that keeps refusing is
-- disconnected instead of being tried every day for ever; one good post or a repeat join resets it.
ALTER TABLE team_hooks ADD COLUMN fails INT NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE team_hooks DROP COLUMN fails;
