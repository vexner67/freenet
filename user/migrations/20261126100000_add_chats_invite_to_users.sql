-- +goose Up
-- "Кто может добавлять меня в чаты" of openvk's privacy settings: the
-- people the owner follows by default, so that a stranger does not pull a
-- page into a group chat.
ALTER TABLE users
    ADD COLUMN chats_invite text NOT NULL DEFAULT 'following'
        CHECK (chats_invite IN ('everyone', 'following', 'nobody'));

-- +goose Down
ALTER TABLE users
    DROP COLUMN chats_invite;
