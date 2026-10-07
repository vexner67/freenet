-- +goose Up
-- What the notification service has to hear, recorded in the transaction
-- of the change and delivered by the relay in id order (pkg/outbox).
CREATE TABLE outbox (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind       text        NOT NULL,
    payload    bytea       NOT NULL,
    request_id text        NOT NULL DEFAULT '',
    attempts   integer     NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    due_at     timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE outbox;
