-- +goose Up
-- Pro members: paid by email through a Razorpay payment link. token is the secret in the member's
-- links (the printable editions); until is when the paid period ends. pro_payments makes a payment
-- count once, because Razorpay sends the same event more than once.
CREATE TABLE pro_members (
    email      TEXT        PRIMARY KEY,
    token      TEXT        NOT NULL UNIQUE,
    until      TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE pro_payments (
    payment_id TEXT        PRIMARY KEY,
    email      TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE pro_payments;
DROP TABLE pro_members;
