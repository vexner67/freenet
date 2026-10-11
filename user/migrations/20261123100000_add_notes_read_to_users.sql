-- +goose Up
-- "Кому видно мои заметки" of openvk's privacy settings: everyone by
-- default, as there. "Мои Заметки" is one more link of the left menu its
-- owner may take away.
ALTER TABLE users
    ADD COLUMN notes_read text NOT NULL DEFAULT 'everyone'
        CHECK (notes_read IN ('everyone', 'users', 'following', 'only_me')),
    DROP CONSTRAINT users_hidden_menu_check,
    ADD CONSTRAINT users_hidden_menu_check
        CHECK (hidden_menu <@ ARRAY['photos', 'messages', 'notes', 'groups', 'events', 'feed', 'notifications']);

-- +goose Down
UPDATE users
SET hidden_menu = array_remove(hidden_menu, 'notes')
WHERE 'notes' = ANY (hidden_menu);

ALTER TABLE users
    DROP COLUMN notes_read,
    DROP CONSTRAINT users_hidden_menu_check,
    ADD CONSTRAINT users_hidden_menu_check
        CHECK (hidden_menu <@ ARRAY['photos', 'messages', 'groups', 'events', 'feed', 'notifications']);
