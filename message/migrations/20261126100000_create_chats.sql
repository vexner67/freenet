-- +goose Up
-- Chats of several people ("беседы"). owner_id, like every user here,
-- refers to the user service's database: no foreign key.
CREATE TABLE chats (
    id         uuid        PRIMARY KEY,
    -- Empty: the chat is called by who is in it.
    title      text        NOT NULL CHECK (char_length(title) <= 64),
    owner_id   uuid        NOT NULL,
    created_at timestamptz NOT NULL
);

-- A message of a chat: what a person wrote, or what happened there
-- (action), then text is the title it tells of and target_id whom it is
-- about. One taken back (deleted_at) says nothing.
CREATE TABLE chat_messages (
    id         uuid        PRIMARY KEY,
    chat_id    uuid        NOT NULL REFERENCES chats (id) ON DELETE CASCADE,
    sender_id  uuid        NOT NULL,
    text       text        NOT NULL,
    photo_ids  uuid[]      NOT NULL DEFAULT '{}' CHECK (cardinality(photo_ids) <= 10),
    action     text        NOT NULL DEFAULT ''
        CHECK (action IN ('', 'create', 'title', 'invite', 'kick', 'leave', 'return')),
    target_id  uuid,
    created_at timestamptz NOT NULL,
    edited_at  timestamptz,
    deleted_at timestamptz,
    CHECK (action <> '' OR deleted_at IS NOT NULL OR text <> '' OR cardinality(photo_ids) > 0),
    CHECK (action = '' OR cardinality(photo_ids) = 0)
);

-- A chat is read by keyset in both directions: id < $before, id > $after.
CREATE INDEX chat_messages_chat_idx ON chat_messages (chat_id, id);
-- What a user whose page is gone wrote, and what was done to them.
CREATE INDEX chat_messages_sender_idx ON chat_messages (sender_id);
CREATE INDEX chat_messages_target_idx ON chat_messages (target_id) WHERE target_id IS NOT NULL;

-- One row per person of a chat, now or before: what the list of chats
-- shows them and how far they have read, as dialogs. Only the rows of
-- those in the chat (active) follow its messages: who left or was removed
-- keeps the chat as it was. The zero UUID means "none".
CREATE TABLE chat_members (
    chat_id          uuid        NOT NULL REFERENCES chats (id) ON DELETE CASCADE,
    user_id          uuid        NOT NULL,
    state            text        NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'left', 'kicked')),
    joined_at        timestamptz NOT NULL,
    last_message_id  uuid        NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    last_incoming_id uuid        NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    read_up_to       uuid        NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    PRIMARY KEY (chat_id, user_id)
);

-- The list of a user's chats, most recently written first, by keyset.
CREATE INDEX chat_members_user_last_message_idx ON chat_members (user_id, last_message_id DESC);

-- +goose Down
DROP TABLE chat_members;
DROP TABLE chat_messages;
DROP TABLE chats;
