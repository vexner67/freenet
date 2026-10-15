-- +goose Up
-- document_ids: files of "Мои Документы" the message attaches, which the
-- post service keeps (no foreign key: a deleted document leaves the
-- message). They are content enough, but not of an action and not with a
-- sticker; those of a forwarded message go in its JSON.
ALTER TABLE messages
    ADD COLUMN document_ids uuid[] NOT NULL DEFAULT '{}' CHECK (cardinality(document_ids) <= 10),
    DROP CONSTRAINT messages_content_check,
    ADD CONSTRAINT messages_content_check CHECK (
        deleted_at IS NOT NULL
        OR sealed IS NOT NULL
        OR cardinality(photo_ids) > 0
        OR post_id IS NOT NULL
        OR shared_photo_id IS NOT NULL
        OR jsonb_array_length(forwarded) > 0
        OR sticker <> ''
        OR cardinality(document_ids) > 0
    ),
    ADD CONSTRAINT messages_documents_check CHECK (sticker = '' OR cardinality(document_ids) = 0);

ALTER TABLE chat_messages
    ADD COLUMN document_ids uuid[] NOT NULL DEFAULT '{}' CHECK (cardinality(document_ids) <= 10),
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
        OR cardinality(document_ids) > 0
    ),
    ADD CONSTRAINT chat_messages_documents_check CHECK ((action = '' AND sticker = '') OR cardinality(document_ids) = 0);

-- +goose Down
-- A message of documents alone would say nothing, so it goes; the dialogs
-- and chats it was the last message or the last incoming one of fall back
-- to the one before it that each side still has, as after a purge.
CREATE TEMPORARY TABLE documented ON COMMIT DROP AS
SELECT id
FROM messages
WHERE deleted_at IS NULL
    AND sealed IS NULL
    AND cardinality(photo_ids) = 0
    AND post_id IS NULL
    AND shared_photo_id IS NULL
    AND forwarded = '[]'
    AND sticker = ''
    AND cardinality(document_ids) > 0;

UPDATE dialogs d
SET last_message_id = COALESCE((
        SELECT m.id
        FROM messages m
        WHERE m.user_lo = LEAST(d.user_id, d.peer_id)
            AND m.user_hi = GREATEST(d.user_id, d.peer_id)
            AND m.id NOT IN (SELECT id FROM documented)
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
            AND m.id NOT IN (SELECT id FROM documented)
            AND m.id <= d.last_incoming_id
            AND m.sender_id = d.peer_id
            AND NOT m.hidden_from_recipient
        ORDER BY m.id DESC
        LIMIT 1
    ), '00000000-0000-0000-0000-000000000000')
WHERE d.last_message_id IN (SELECT id FROM documented)
    OR d.last_incoming_id IN (SELECT id FROM documented);

DELETE FROM messages
WHERE id IN (SELECT id FROM documented);

CREATE TEMPORARY TABLE chat_documented ON COMMIT DROP AS
SELECT id
FROM chat_messages
WHERE action = ''
    AND deleted_at IS NULL
    AND text = ''
    AND cardinality(photo_ids) = 0
    AND post_id IS NULL
    AND shared_photo_id IS NULL
    AND forwarded = '[]'
    AND sticker = ''
    AND cardinality(document_ids) > 0;

UPDATE chat_members cm
SET last_message_id = COALESCE((
        SELECT m.id
        FROM chat_messages m
        WHERE m.chat_id = cm.chat_id
            AND m.id NOT IN (SELECT id FROM chat_documented)
            AND m.id <= cm.last_message_id
        ORDER BY m.id DESC
        LIMIT 1
    ), '00000000-0000-0000-0000-000000000000'),
    last_incoming_id = COALESCE((
        SELECT m.id
        FROM chat_messages m
        WHERE m.chat_id = cm.chat_id
            AND m.id NOT IN (SELECT id FROM chat_documented)
            AND m.id <= cm.last_incoming_id
            AND m.sender_id <> cm.user_id
        ORDER BY m.id DESC
        LIMIT 1
    ), '00000000-0000-0000-0000-000000000000')
WHERE cm.last_message_id IN (SELECT id FROM chat_documented)
    OR cm.last_incoming_id IN (SELECT id FROM chat_documented);

DELETE FROM chat_messages
WHERE id IN (SELECT id FROM chat_documented);

ALTER TABLE chat_messages
    DROP CONSTRAINT chat_messages_documents_check,
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
    DROP COLUMN document_ids;

ALTER TABLE messages
    DROP CONSTRAINT messages_documents_check,
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
    DROP COLUMN document_ids;
