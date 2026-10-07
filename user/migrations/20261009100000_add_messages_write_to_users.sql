-- +goose Up
-- Who may write private messages to the user. Existing users keep the
-- open messages they had.
ALTER TABLE users
    ADD COLUMN messages_write text NOT NULL DEFAULT 'everyone'
        CHECK (messages_write IN ('everyone', 'following', 'nobody'));

-- +goose Down
ALTER TABLE users
    DROP COLUMN messages_write;
