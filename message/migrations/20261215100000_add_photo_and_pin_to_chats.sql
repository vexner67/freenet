-- +goose Up
-- photo_id: the photo of a chat, which the media service keeps under the
-- chat's ID (no foreign key). pinned_message_id: the message pinned above
-- the chat ("Закреплённое сообщение"); one gone with its sender's page
-- is pinned no more. Setting and removing the photo, pinning and
-- unpinning are told to the chat as its other actions.
ALTER TABLE chats
    ADD COLUMN photo_id uuid,
    ADD COLUMN pinned_message_id uuid REFERENCES chat_messages (id) ON DELETE SET NULL;

ALTER TABLE chat_messages
    DROP CONSTRAINT chat_messages_action_check,
    ADD CONSTRAINT chat_messages_action_check
        CHECK (action IN ('', 'create', 'title', 'invite', 'kick', 'leave', 'return', 'pin', 'unpin', 'photo', 'photo_remove'));

-- +goose Down
-- The new actions go; who had one as the last message or the last
-- incoming one falls back to the one before it, as after a purge.
CREATE TEMPORARY TABLE new_actions ON COMMIT DROP AS
SELECT id
FROM chat_messages
WHERE action IN ('pin', 'unpin', 'photo', 'photo_remove');

UPDATE chat_members cm
SET last_message_id = COALESCE((
        SELECT m.id
        FROM chat_messages m
        WHERE m.chat_id = cm.chat_id
            AND m.id NOT IN (SELECT id FROM new_actions)
            AND m.id <= cm.last_message_id
        ORDER BY m.id DESC
        LIMIT 1
    ), '00000000-0000-0000-0000-000000000000'),
    last_incoming_id = COALESCE((
        SELECT m.id
        FROM chat_messages m
        WHERE m.chat_id = cm.chat_id
            AND m.id NOT IN (SELECT id FROM new_actions)
            AND m.id <= cm.last_incoming_id
            AND m.sender_id <> cm.user_id
        ORDER BY m.id DESC
        LIMIT 1
    ), '00000000-0000-0000-0000-000000000000')
WHERE cm.last_message_id IN (SELECT id FROM new_actions)
    OR cm.last_incoming_id IN (SELECT id FROM new_actions);

DELETE FROM chat_messages
WHERE id IN (SELECT id FROM new_actions);

ALTER TABLE chat_messages
    DROP CONSTRAINT chat_messages_action_check,
    ADD CONSTRAINT chat_messages_action_check
        CHECK (action IN ('', 'create', 'title', 'invite', 'kick', 'leave', 'return'));

ALTER TABLE chats
    DROP COLUMN pinned_message_id,
    DROP COLUMN photo_id;
