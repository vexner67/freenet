-- +goose Up
-- openvk's "Закладки" read what a person likes, newest like first. The
-- indexes also find a user's likes to erase them, as the ones by user_id
-- alone did.
CREATE INDEX post_likes_faves_idx ON post_likes (user_id, created_at DESC, post_id DESC);
CREATE INDEX comment_likes_faves_idx ON comment_likes (user_id, created_at DESC, comment_id DESC);
CREATE INDEX album_photo_likes_faves_idx ON album_photo_likes (user_id, created_at DESC, photo_id DESC);
DROP INDEX post_likes_user_id_idx;
DROP INDEX comment_likes_user_id_idx;
DROP INDEX album_photo_likes_user_id_idx;

-- +goose Down
CREATE INDEX album_photo_likes_user_id_idx ON album_photo_likes (user_id);
CREATE INDEX comment_likes_user_id_idx ON comment_likes (user_id);
CREATE INDEX post_likes_user_id_idx ON post_likes (user_id);
DROP INDEX album_photo_likes_faves_idx;
DROP INDEX comment_likes_faves_idx;
DROP INDEX post_likes_faves_idx;
