-- +goose Up
-- A message to oneself goes to «Избранное», the dialog where a user keeps
-- what they want to keep. The inline check has a generated name: find it.
-- +goose StatementBegin
DO $$
DECLARE
    name text;
BEGIN
    SELECT conname INTO name
    FROM pg_constraint
    WHERE conrelid = 'messages'::regclass
        AND contype = 'c'
        AND pg_get_constraintdef(oid) LIKE '%recipient_id <> sender_id%';
    IF name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE messages DROP CONSTRAINT %I', name);
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
DELETE FROM messages WHERE recipient_id = sender_id;
DELETE FROM dialogs WHERE user_id = peer_id;
ALTER TABLE messages ADD CHECK (recipient_id <> sender_id);
