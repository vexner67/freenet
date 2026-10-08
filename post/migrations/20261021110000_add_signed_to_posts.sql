-- +goose Up
-- A post that speaks for a community may name its author ("Подпись
-- автора"); only such a post.
ALTER TABLE posts
    ADD COLUMN signed boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT posts_signed_check CHECK (NOT signed OR from_wall);

-- +goose Down
ALTER TABLE posts
    DROP COLUMN signed;
