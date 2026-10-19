-- +goose Up
-- Notes ("Заметки") are gone from the site, and so are "Кому видно мои
-- заметки" and their link of the left menu.
UPDATE users
SET hidden_menu = array_remove(hidden_menu, 'notes')
WHERE 'notes' = ANY (hidden_menu);

ALTER TABLE users
    DROP COLUMN notes_read,
    DROP CONSTRAINT users_hidden_menu_check,
    ADD CONSTRAINT users_hidden_menu_check
        CHECK (hidden_menu <@ ARRAY['photos', 'messages', 'groups', 'feed', 'notifications']);

-- +goose Down
ALTER TABLE users
    ADD COLUMN notes_read text NOT NULL DEFAULT 'everyone'
        CHECK (notes_read IN ('everyone', 'users', 'following', 'only_me')),
    DROP CONSTRAINT users_hidden_menu_check,
    ADD CONSTRAINT users_hidden_menu_check
        CHECK (hidden_menu <@ ARRAY['photos', 'messages', 'notes', 'groups', 'feed', 'notifications']);
