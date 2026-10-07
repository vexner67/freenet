-- +goose Up
-- One stream per receiving service, each with its own order and relay;
-- what was recorded so far is for notification.
ALTER TABLE outbox
    ADD COLUMN stream text NOT NULL DEFAULT 'notification';

ALTER TABLE outbox
    ALTER COLUMN stream DROP DEFAULT;

CREATE INDEX outbox_stream_idx ON outbox (stream, id);

-- +goose Down
-- Only notifications were in the outbox before.
DELETE FROM outbox WHERE stream <> 'notification';

DROP INDEX outbox_stream_idx;

ALTER TABLE outbox
    DROP COLUMN stream;
