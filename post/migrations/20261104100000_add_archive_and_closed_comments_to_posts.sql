-- +goose Up
-- archived: the owner of the wall put the post away ("Архив записей" of
-- openvk), no wall or feed shows it. comments_closed: the owner lets
-- nobody comment it anymore.
ALTER TABLE posts
    ADD COLUMN archived        boolean NOT NULL DEFAULT false,
    ADD COLUMN comments_closed boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE posts
    DROP COLUMN comments_closed,
    DROP COLUMN archived;
