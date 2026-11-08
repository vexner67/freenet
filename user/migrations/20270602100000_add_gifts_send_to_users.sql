-- +goose Up
-- "Кто может отправлять мне подарки" of openvk: everyone, the friends or
-- nobody; everyone, as before the setting.
ALTER TABLE users
    ADD COLUMN gifts_send text NOT NULL DEFAULT 'everyone'
        CHECK (gifts_send IN ('everyone', 'following', 'nobody'));

-- +goose Down
ALTER TABLE users
    DROP COLUMN gifts_send;
