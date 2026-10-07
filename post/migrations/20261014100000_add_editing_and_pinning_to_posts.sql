-- +goose Up
-- A post edited by its author shows "(ред.)".
ALTER TABLE posts
    ADD COLUMN edited_at timestamptz;

-- The post the owner of a wall shows first on it: one per wall, so pinning
-- another replaces it in one statement (ON CONFLICT), and a deleted post
-- takes its pin with it.
CREATE TABLE pinned_posts (
    wall_id uuid PRIMARY KEY,
    post_id uuid NOT NULL UNIQUE REFERENCES posts (id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE pinned_posts;

ALTER TABLE posts
    DROP COLUMN edited_at;
