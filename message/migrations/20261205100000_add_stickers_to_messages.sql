-- +goose Up
-- sticker: a sticker of the catalogue the user service keeps ("Стикеры"
-- of openvk), '' for none. A message that is a sticker is nothing else,
-- as on VK; the sticker of a forwarded one goes in its JSON.
ALTER TABLE messages
    ADD COLUMN sticker text NOT NULL DEFAULT '' CHECK (sticker ~ '^[a-z0-9_]{0,32}$'),
    DROP CONSTRAINT messages_content_check,
    ADD CONSTRAINT messages_content_check CHECK (
        deleted_at IS NOT NULL
        OR sealed IS NOT NULL
        OR cardinality(photo_ids) > 0
        OR post_id IS NOT NULL
        OR shared_photo_id IS NOT NULL
        OR jsonb_array_length(forwarded) > 0
        OR sticker <> ''
    ),
    ADD CONSTRAINT messages_sticker_alone_check CHECK (
        sticker = ''
        OR (sealed IS NULL AND cardinality(photo_ids) = 0 AND post_id IS NULL AND shared_photo_id IS NULL AND forwarded = '[]')
    );

ALTER TABLE chat_messages
    ADD COLUMN sticker text NOT NULL DEFAULT '' CHECK (sticker ~ '^[a-z0-9_]{0,32}$'),
    DROP CONSTRAINT chat_messages_content_check,
    ADD CONSTRAINT chat_messages_content_check CHECK (
        action <> ''
        OR deleted_at IS NOT NULL
        OR text <> ''
        OR cardinality(photo_ids) > 0
        OR post_id IS NOT NULL
        OR shared_photo_id IS NOT NULL
        OR forwarded <> '[]'
        OR sticker <> ''
    ),
    ADD CONSTRAINT chat_messages_sticker_alone_check CHECK (
        sticker = ''
        OR (action = '' AND text = '' AND cardinality(photo_ids) = 0 AND post_id IS NULL AND shared_photo_id IS NULL AND forwarded = '[]')
    );

-- +goose Down
-- A message that was a sticker would say nothing, so it goes; the dialogs
-- and chats it was the last message or the last incoming one of fall back
-- to the one before it that each side still has, as after a purge.
UPDATE dialogs d
SET last_message_id = COALESCE((
        SELECT m.id
        FROM messages m
        WHERE m.user_lo = LEAST(d.user_id, d.peer_id)
            AND m.user_hi = GREATEST(d.user_id, d.peer_id)
            AND m.sticker = ''
            AND m.id <= d.last_message_id
            AND NOT (m.sender_id = d.user_id AND m.hidden_from_sender)
            AND NOT (m.recipient_id = d.user_id AND m.hidden_from_recipient)
        ORDER BY m.id DESC
        LIMIT 1
    ), '00000000-0000-0000-0000-000000000000'),
    last_incoming_id = COALESCE((
        SELECT m.id
        FROM messages m
        WHERE m.user_lo = LEAST(d.user_id, d.peer_id)
            AND m.user_hi = GREATEST(d.user_id, d.peer_id)
            AND m.sticker = ''
            AND m.id <= d.last_incoming_id
            AND m.sender_id = d.peer_id
            AND NOT m.hidden_from_recipient
        ORDER BY m.id DESC
        LIMIT 1
    ), '00000000-0000-0000-0000-000000000000')
WHERE d.last_message_id IN (SELECT id FROM messages WHERE sticker <> '')
    OR d.last_incoming_id IN (SELECT id FROM messages WHERE sticker <> '');

DELETE FROM messages
WHERE sticker <> '';

UPDATE chat_members cm
SET last_message_id = COALESCE((
        SELECT m.id
        FROM chat_messages m
        WHERE m.chat_id = cm.chat_id
            AND m.sticker = ''
            AND m.id <= cm.last_message_id
        ORDER BY m.id DESC
        LIMIT 1
    ), '00000000-0000-0000-0000-000000000000'),
    last_incoming_id = COALESCE((
        SELECT m.id
        FROM chat_messages m
        WHERE m.chat_id = cm.chat_id
            AND m.sticker = ''
            AND m.id <= cm.last_incoming_id
            AND m.sender_id <> cm.user_id
        ORDER BY m.id DESC
        LIMIT 1
    ), '00000000-0000-0000-0000-000000000000')
WHERE cm.last_message_id IN (SELECT id FROM chat_messages WHERE sticker <> '')
    OR cm.last_incoming_id IN (SELECT id FROM chat_messages WHERE sticker <> '');

DELETE FROM chat_messages
WHERE sticker <> '';

ALTER TABLE chat_messages
    DROP CONSTRAINT chat_messages_sticker_alone_check,
    DROP CONSTRAINT chat_messages_content_check,
    ADD CONSTRAINT chat_messages_content_check CHECK (
        action <> ''
        OR deleted_at IS NOT NULL
        OR text <> ''
        OR cardinality(photo_ids) > 0
        OR post_id IS NOT NULL
        OR shared_photo_id IS NOT NULL
        OR forwarded <> '[]'
    ),
    DROP COLUMN sticker;

ALTER TABLE messages
    DROP CONSTRAINT messages_sticker_alone_check,
    DROP CONSTRAINT messages_content_check,
    ADD CONSTRAINT messages_content_check CHECK (
        deleted_at IS NOT NULL
        OR sealed IS NOT NULL
        OR cardinality(photo_ids) > 0
        OR post_id IS NOT NULL
        OR shared_photo_id IS NOT NULL
        OR jsonb_array_length(forwarded) > 0
    ),
    DROP COLUMN sticker;
