-- +goose Up
-- When a person last visited the site (openvk's "Онлайн", "был в сети"),
-- NULL if never, and "Кто видит мою неактивность": who sees a long
-- absence; everyone by default, as there.
ALTER TABLE users
    ADD COLUMN last_seen_at timestamptz,
    ADD COLUMN online_read text NOT NULL DEFAULT 'everyone'
        CHECK (online_read IN ('everyone', 'users', 'following', 'only_me'));

-- +goose Down
ALTER TABLE users
    DROP COLUMN online_read,
    DROP COLUMN last_seen_at;
