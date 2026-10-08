-- +goose Up
-- "Кому видно мои группы" of openvk; everyone by default, as there.
ALTER TABLE users
    ADD COLUMN clubs_read text NOT NULL DEFAULT 'everyone'
        CHECK (clubs_read IN ('everyone', 'following', 'only_me'));

-- +goose Down
ALTER TABLE users
    DROP COLUMN clubs_read;
