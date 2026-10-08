-- +goose Up
-- A post on a community's wall may speak for the community rather than
-- for its author ("От имени сообщества"); never on the author's own wall.
ALTER TABLE posts
    ADD COLUMN from_wall boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT posts_from_wall_check CHECK (NOT from_wall OR wall_id <> author_id);

-- +goose Down
ALTER TABLE posts
    DROP COLUMN from_wall;
