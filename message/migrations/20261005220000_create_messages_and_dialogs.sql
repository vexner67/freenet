-- +goose Up
-- sender_id and recipient_id refer to the user service's database, so
-- there are no foreign keys: the gateway checks the recipient exists.
CREATE TABLE messages (
    id           uuid        PRIMARY KEY,
    sender_id    uuid        NOT NULL,
    recipient_id uuid        NOT NULL CHECK (recipient_id <> sender_id),
    -- The pair of people, in one order whoever writes: a dialog is one
    -- range of the index below.
    user_lo      uuid        NOT NULL GENERATED ALWAYS AS (LEAST(sender_id, recipient_id)) STORED,
    user_hi      uuid        NOT NULL GENERATED ALWAYS AS (GREATEST(sender_id, recipient_id)) STORED,
    text         text        NOT NULL CHECK (text <> ''),
    created_at   timestamptz NOT NULL
);

-- A dialog is read by keyset in both directions: id < $before, id > $after.
CREATE INDEX messages_dialog_idx ON messages (user_lo, user_hi, id);

-- One row per side of a dialog: what the list of dialogs shows and how
-- far the user has read. Updated in the same statement as the message.
-- The zero UUID means "none" and sorts below every real one.
CREATE TABLE dialogs (
    user_id          uuid NOT NULL,
    peer_id          uuid NOT NULL,
    last_message_id  uuid NOT NULL,
    last_incoming_id uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    read_up_to       uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    PRIMARY KEY (user_id, peer_id)
);

-- The list of dialogs, most recently written first, by keyset.
CREATE INDEX dialogs_user_last_message_idx ON dialogs (user_id, last_message_id DESC);

-- +goose Down
DROP TABLE dialogs;
DROP TABLE messages;
