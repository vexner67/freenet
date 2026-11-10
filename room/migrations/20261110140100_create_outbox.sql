-- +goose Up
CREATE TABLE outbox (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    stream     text        NOT NULL,
    kind       text        NOT NULL,
    payload    bytea       NOT NULL,
    request_id text        NOT NULL DEFAULT '',
    attempts   integer     NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    due_at     timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX outbox_stream_idx ON outbox (stream, id);

-- +goose Down
DROP TABLE outbox;
