-- +goose Up
-- forwarded: the messages a message carries on from other dialogs
-- ("Переслать"), copies of what they showed: a JSON array of
-- {sender_id, text, photos, post, created_at}. deleted_at: the sender
-- took the message back from both sides; the row stays in the dialog and
-- says nothing ("Сообщение удалено"), so only such a message may be empty.
ALTER TABLE messages
    ADD COLUMN forwarded  jsonb NOT NULL DEFAULT '[]',
    ADD COLUMN deleted_at timestamptz,
    ADD CONSTRAINT messages_forwarded_check CHECK (jsonb_typeof(forwarded) = 'array' AND jsonb_array_length(forwarded) <= 20),
    DROP CONSTRAINT messages_content_check,
    ADD CONSTRAINT messages_content_check CHECK (
        deleted_at IS NOT NULL
        OR text <> ''
        OR cardinality(photo_ids) > 0
        OR post_id IS NOT NULL
        OR jsonb_array_length(forwarded) > 0
    );

-- +goose Down
-- Messages that say nothing without the new columns go, and the dialogs
-- that ended with them go back to what is left.
DELETE FROM messages
WHERE deleted_at IS NOT NULL
    OR (text = '' AND cardinality(photo_ids) = 0 AND post_id IS NULL);

UPDATE dialogs d
SET last_message_id = COALESCE((
        SELECT m.id
        FROM messages m
        WHERE m.user_lo = LEAST(d.user_id, d.peer_id)
            AND m.user_hi = GREATEST(d.user_id, d.peer_id)
            AND NOT CASE WHEN m.sender_id = d.user_id THEN m.hidden_from_sender ELSE m.hidden_from_recipient END
        ORDER BY m.id DESC
        LIMIT 1
    ), '00000000-0000-0000-0000-000000000000'),
    last_incoming_id = COALESCE((
        SELECT m.id
        FROM messages m
        WHERE m.user_lo = LEAST(d.user_id, d.peer_id)
            AND m.user_hi = GREATEST(d.user_id, d.peer_id)
            AND m.sender_id = d.peer_id
            AND NOT m.hidden_from_recipient
        ORDER BY m.id DESC
        LIMIT 1
    ), '00000000-0000-0000-0000-000000000000')
WHERE NOT EXISTS (SELECT 1 FROM messages m WHERE m.id = d.last_message_id)
    OR (d.last_incoming_id <> '00000000-0000-0000-0000-000000000000'
        AND NOT EXISTS (SELECT 1 FROM messages m WHERE m.id = d.last_incoming_id));

ALTER TABLE messages
    DROP CONSTRAINT messages_content_check,
    ADD CONSTRAINT messages_content_check CHECK (text <> '' OR cardinality(photo_ids) > 0 OR post_id IS NOT NULL),
    DROP CONSTRAINT messages_forwarded_check,
    DROP COLUMN deleted_at,
    DROP COLUMN forwarded;
