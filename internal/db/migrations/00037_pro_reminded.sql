-- +goose Up
-- The paid period a reminder mail was last sent for. A member is reminded once per period: a renewal moves "until",
-- so the stored value no longer matches and the next period gets its own reminder.
ALTER TABLE pro_members ADD COLUMN reminded_until TIMESTAMPTZ;

-- +goose Down
ALTER TABLE pro_members DROP COLUMN reminded_until;
