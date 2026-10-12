-- +goose Up
-- A message of a chat may be what a message of a dialog is: it shares a
-- post or a photo of an album and carries other messages on (forwarded,
-- as in messages: copies, a JSON array).
ALTER TABLE chat_messages
    ADD COLUMN post_id         uuid,
    ADD COLUMN shared_photo_id uuid,
    ADD COLUMN forwarded       jsonb NOT NULL DEFAULT '[]'
        CHECK (jsonb_typeof(forwarded) = 'array' AND jsonb_array_length(forwarded) <= 20);

ALTER TABLE chat_messages
    DROP CONSTRAINT chat_messages_check,
    DROP CONSTRAINT chat_messages_check1,
    ADD CONSTRAINT chat_messages_content_check CHECK (
        action <> ''
        OR deleted_at IS NOT NULL
        OR text <> ''
        OR cardinality(photo_ids) > 0
        OR post_id IS NOT NULL
        OR shared_photo_id IS NOT NULL
        OR forwarded <> '[]'
    ),
    ADD CONSTRAINT chat_messages_action_content_check CHECK (
        action = ''
        OR (cardinality(photo_ids) = 0 AND post_id IS NULL AND shared_photo_id IS NULL AND forwarded = '[]')
    );

-- The search of chats, as that of dialogs (messages_search_idx): a
-- substring of what a message says itself or in those it carries on.
CREATE INDEX chat_messages_search_idx ON chat_messages USING gin (chat_id, message_words(text, forwarded) gin_trgm_ops);

-- +goose Down
DROP INDEX chat_messages_search_idx;

-- A message that only shared or carried something on would say nothing.
DELETE FROM chat_messages
WHERE action = ''
    AND deleted_at IS NULL
    AND text = ''
    AND cardinality(photo_ids) = 0;

UPDATE chat_members m
SET last_message_id = COALESCE((
        SELECT msg.id
        FROM chat_messages msg
        WHERE msg.chat_id = m.chat_id
            AND msg.id <= m.last_message_id
        ORDER BY msg.id DESC
        LIMIT 1
    ), '00000000-0000-0000-0000-000000000000'),
    last_incoming_id = COALESCE((
        SELECT msg.id
        FROM chat_messages msg
        WHERE msg.chat_id = m.chat_id
            AND msg.id <= m.last_incoming_id
            AND msg.sender_id <> m.user_id
        ORDER BY msg.id DESC
        LIMIT 1
    ), '00000000-0000-0000-0000-000000000000');

ALTER TABLE chat_messages
    DROP CONSTRAINT chat_messages_content_check,
    DROP CONSTRAINT chat_messages_action_content_check,
    DROP COLUMN post_id,
    DROP COLUMN shared_photo_id,
    DROP COLUMN forwarded,
    ADD CHECK (action <> '' OR deleted_at IS NOT NULL OR text <> '' OR cardinality(photo_ids) > 0),
    ADD CHECK (action = '' OR cardinality(photo_ids) = 0);
