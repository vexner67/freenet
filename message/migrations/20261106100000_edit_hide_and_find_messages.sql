-- +goose Up
-- edited_at: the sender changed what the message says. hidden_from_*: that
-- side deleted the message for themselves ("Удалить у себя"), the other
-- still has it; once both have, the row goes.
ALTER TABLE messages
    ADD COLUMN edited_at             timestamptz,
    ADD COLUMN hidden_from_sender    boolean NOT NULL DEFAULT false,
    ADD COLUMN hidden_from_recipient boolean NOT NULL DEFAULT false;

-- The search of all the dialogs of a user reads their messages by either
-- side, newest first.
CREATE INDEX messages_sender_idx ON messages (sender_id, id DESC);
CREATE INDEX messages_recipient_idx ON messages (recipient_id, id DESC);

-- +goose Down
DROP INDEX messages_recipient_idx;
DROP INDEX messages_sender_idx;
ALTER TABLE messages
    DROP COLUMN hidden_from_recipient,
    DROP COLUMN hidden_from_sender,
    DROP COLUMN edited_at;
