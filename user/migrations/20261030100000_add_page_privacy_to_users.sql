-- +goose Up
-- openvk's "Кому в интернете видно мою страницу" (everyone or signed-in
-- users), "Кому видно основную информацию моей страницы", "… мои
-- фотографии" and "… моих друзей" (the follows here); everyone by
-- default, as there. "Кому видно мои группы" may name users too.
ALTER TABLE users
    ADD COLUMN page_read text NOT NULL DEFAULT 'everyone'
        CHECK (page_read IN ('everyone', 'users')),
    ADD COLUMN info_read text NOT NULL DEFAULT 'everyone'
        CHECK (info_read IN ('everyone', 'users', 'following', 'only_me')),
    ADD COLUMN photos_read text NOT NULL DEFAULT 'everyone'
        CHECK (photos_read IN ('everyone', 'users', 'following', 'only_me')),
    ADD COLUMN friends_read text NOT NULL DEFAULT 'everyone'
        CHECK (friends_read IN ('everyone', 'users', 'following', 'only_me')),
    DROP CONSTRAINT users_clubs_read_check,
    ADD CONSTRAINT users_clubs_read_check CHECK (clubs_read IN ('everyone', 'users', 'following', 'only_me'));

-- +goose Down
UPDATE users SET clubs_read = 'everyone' WHERE clubs_read = 'users';
ALTER TABLE users
    DROP CONSTRAINT users_clubs_read_check,
    ADD CONSTRAINT users_clubs_read_check CHECK (clubs_read IN ('everyone', 'following', 'only_me')),
    DROP COLUMN friends_read,
    DROP COLUMN photos_read,
    DROP COLUMN info_read,
    DROP COLUMN page_read;
