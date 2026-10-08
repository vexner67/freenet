-- +goose Up
-- photo_ids refer to the media service, so there is no foreign key: the
-- gateway checks they are the sender's.
ALTER TABLE messages
    ADD COLUMN photo_ids uuid[] NOT NULL DEFAULT '{}' CHECK (cardinality(photo_ids) <= 10);

-- A message may now be photos only, but never empty.
ALTER TABLE messages
    DROP CONSTRAINT messages_text_check,
    ADD CONSTRAINT messages_content_check CHECK (text <> '' OR cardinality(photo_ids) > 0);

-- What the media service has to hear about the photos of messages,
-- recorded in the transaction of the change and delivered by the relay of
-- each stream in id order (pkg/outbox).
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

-- Messages of photos only keep their place in the dialogs, saying so.
UPDATE messages SET text = 'Фотография' WHERE text = '';

ALTER TABLE messages
    DROP CONSTRAINT messages_content_check,
    ADD CONSTRAINT messages_text_check CHECK (text <> '');

ALTER TABLE messages
    DROP COLUMN photo_ids;
