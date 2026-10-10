-- +goose Up
-- "Интерфейс" of the settings: how the site looks to its owner alone.
-- hidden_menu are the links taken out of the left menu.
ALTER TABLE users
    ADD COLUMN avatar_style text   NOT NULL DEFAULT 'square' CHECK (avatar_style IN ('square', 'round')),
    ADD COLUMN main_page    text   NOT NULL DEFAULT 'page' CHECK (main_page IN ('page', 'feed')),
    ADD COLUMN hidden_menu  text[] NOT NULL DEFAULT '{}'
        CHECK (hidden_menu <@ ARRAY['photos', 'messages', 'groups', 'events', 'feed', 'notifications']);

-- +goose Down
ALTER TABLE users
    DROP COLUMN hidden_menu,
    DROP COLUMN main_page,
    DROP COLUMN avatar_style;
