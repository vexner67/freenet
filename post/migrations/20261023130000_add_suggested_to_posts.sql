-- +goose Up
-- A post suggested for a community's wall and published by its admins:
-- the community's now, which its author neither edits nor deletes.
ALTER TABLE posts
    ADD COLUMN suggested boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT posts_suggested_check CHECK (NOT suggested OR from_wall);

-- +goose Down
ALTER TABLE posts
    DROP CONSTRAINT posts_suggested_check,
    DROP COLUMN suggested;
