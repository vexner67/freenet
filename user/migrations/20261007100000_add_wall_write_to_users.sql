-- +goose Up
-- Who may write on the user's wall besides the user. Existing walls get the
-- default of new ones: the people the owner follows.
ALTER TABLE users
    ADD COLUMN wall_write text NOT NULL DEFAULT 'following'
        CHECK (wall_write IN ('everyone', 'following', 'only_me'));

-- +goose Down
ALTER TABLE users
    DROP COLUMN wall_write;
