-- +goose Up
-- Erasing a user finds their likes and comments on others' posts.
CREATE INDEX post_likes_user_id_idx ON post_likes (user_id);
CREATE INDEX comments_author_id_idx ON comments (author_id);

-- +goose Down
DROP INDEX comments_author_id_idx;
DROP INDEX post_likes_user_id_idx;
