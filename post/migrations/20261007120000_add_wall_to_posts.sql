-- +goose Up
-- A post is written by author_id on the wall of wall_id: the author's own
-- or someone else's. wall_id refers to the user service's database, so
-- there is no foreign key. Every post so far is on its author's wall.
ALTER TABLE posts
    ADD COLUMN wall_id uuid;

UPDATE posts SET wall_id = author_id;

ALTER TABLE posts
    ALTER COLUMN wall_id SET NOT NULL;

-- A wall is read newest first by keyset: wall_id = $1 AND id < $cursor.
-- posts_author_id_idx stays for news feeds, which are by author.
CREATE INDEX posts_wall_id_idx ON posts (wall_id, id DESC);

-- +goose Down
-- Posts on others' walls cannot exist without the column.
DELETE FROM posts WHERE wall_id <> author_id;

ALTER TABLE posts
    DROP COLUMN wall_id;
