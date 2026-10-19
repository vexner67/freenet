-- +goose Up
-- A block of a page by the administrators of the site ("Заблокировать
-- пользователя" of openvk): since when, until when (NULL: for good) and
-- why. A page that was never blocked, or was let be, has none of them.
ALTER TABLE users
    ADD COLUMN banned_at timestamptz,
    ADD COLUMN banned_until timestamptz,
    ADD COLUMN ban_reason text NOT NULL DEFAULT '',
    ADD CONSTRAINT users_ban_check CHECK (
        (banned_at IS NULL AND banned_until IS NULL AND ban_reason = '')
        OR (banned_at IS NOT NULL
            AND char_length(ban_reason) BETWEEN 1 AND 255
            AND (banned_until IS NULL OR banned_until > banned_at))
    );

-- The administrators' list of the blocked pages, newest first.
CREATE INDEX users_banned_idx ON users (id DESC) WHERE banned_at IS NOT NULL;

-- +goose Down
DROP INDEX users_banned_idx;

ALTER TABLE users
    DROP CONSTRAINT users_ban_check,
    DROP COLUMN ban_reason,
    DROP COLUMN banned_until,
    DROP COLUMN banned_at;
