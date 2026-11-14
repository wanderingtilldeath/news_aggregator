-- +goose Up
-- One row per subscriber per edition day: the daily send records here after the
-- mail is accepted, so running it twice on a day never mails anyone twice.
CREATE TABLE deliveries (
    subscriber_id BIGINT      NOT NULL REFERENCES subscribers (id) ON DELETE CASCADE,
    edition_day   DATE        NOT NULL,
    sent_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (subscriber_id, edition_day)
);

-- +goose Down
DROP TABLE deliveries;
