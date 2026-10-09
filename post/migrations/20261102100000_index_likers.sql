-- +goose Up
-- Who liked a post or a comment, newest first, by keyset on (created_at,
-- user_id): openvk's "Понравилось N людям".
CREATE INDEX post_likes_likers_idx ON post_likes (post_id, created_at DESC, user_id DESC);
CREATE INDEX comment_likes_likers_idx ON comment_likes (comment_id, created_at DESC, user_id DESC);

-- +goose Down
DROP INDEX comment_likes_likers_idx;
DROP INDEX post_likes_likers_idx;
