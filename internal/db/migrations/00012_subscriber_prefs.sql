-- +goose Up
-- What each subscriber reads: which editions (tracks) and how long (1min or 7min).
-- Existing subscribers get every track and the one-minute read.
ALTER TABLE subscribers
    ADD COLUMN tracks TEXT[] NOT NULL DEFAULT '{upsc,engineering}',
    ADD COLUMN mode   TEXT   NOT NULL DEFAULT '1min' CHECK (mode IN ('1min', '7min'));

-- +goose Down
ALTER TABLE subscribers DROP COLUMN mode, DROP COLUMN tracks;
