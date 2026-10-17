-- +goose Up
-- Events ("встречи") are gone from the site, and so is their link of the
-- left menu.
UPDATE users
SET hidden_menu = array_remove(hidden_menu, 'events')
WHERE 'events' = ANY (hidden_menu);

ALTER TABLE users
    DROP CONSTRAINT users_hidden_menu_check,
    ADD CONSTRAINT users_hidden_menu_check
        CHECK (hidden_menu <@ ARRAY['photos', 'messages', 'notes', 'groups', 'feed', 'notifications']);

-- +goose Down
ALTER TABLE users
    DROP CONSTRAINT users_hidden_menu_check,
    ADD CONSTRAINT users_hidden_menu_check
        CHECK (hidden_menu <@ ARRAY['photos', 'messages', 'notes', 'groups', 'events', 'feed', 'notifications']);
