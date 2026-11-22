-- +goose Up
-- Every admin write (removing a subscriber, granting or revoking Pro) leaves a row: who, what, on which target.
-- The target is an id or an address the admin typed; the detail never holds a secret.
CREATE TABLE admin_audit (
    id     BIGSERIAL   PRIMARY KEY,
    at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor  TEXT        NOT NULL,
    action TEXT        NOT NULL,
    target TEXT        NOT NULL DEFAULT '',
    detail TEXT        NOT NULL DEFAULT ''
);
CREATE INDEX admin_audit_at_idx ON admin_audit (at DESC);

-- A captured payment whose amount matches no plan: it bought nothing, so the owner sees it here and grants the days
-- by hand. Razorpay may send the same payment twice, hence the primary key.
CREATE TABLE pro_unmatched (
    payment_id TEXT        PRIMARY KEY,
    email      TEXT        NOT NULL,
    amount     INTEGER     NOT NULL, -- paise
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE pro_unmatched;
DROP TABLE admin_audit;
