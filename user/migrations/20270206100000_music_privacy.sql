-- +goose Up
-- "Кто видит мои аудиозаписи" and "Кто видит, что я слушаю": everyone by
-- default, as the songs of a page were shown before.
ALTER TABLE users
    ADD COLUMN audios_read text NOT NULL DEFAULT 'everyone'
        CHECK (audios_read IN ('everyone', 'users', 'following', 'only_me')),
    ADD COLUMN listening_read text NOT NULL DEFAULT 'everyone'
        CHECK (listening_read IN ('everyone', 'users', 'following', 'only_me'));

-- +goose Down
ALTER TABLE users
    DROP COLUMN listening_read,
    DROP COLUMN audios_read;
