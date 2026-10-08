-- +goose Up
-- A reply answers another comment of the same post; a deleted comment
-- leaves its replies, which then answer nothing.
ALTER TABLE comments
    ADD COLUMN reply_to   uuid REFERENCES comments (id) ON DELETE SET NULL,
    ADD COLUMN like_count integer NOT NULL DEFAULT 0 CHECK (like_count >= 0);

-- ON DELETE SET NULL finds the replies of a deleted comment by it.
CREATE INDEX comments_reply_to_idx ON comments (reply_to) WHERE reply_to IS NOT NULL;

-- Likes of comments, as of posts: one per user, counted in like_count by
-- the same statement, deleted with the comment.
CREATE TABLE comment_likes (
    comment_id uuid        NOT NULL REFERENCES comments (id) ON DELETE CASCADE,
    user_id    uuid        NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (comment_id, user_id)
);

-- Erasing a user finds their likes by it.
CREATE INDEX comment_likes_user_id_idx ON comment_likes (user_id);

-- +goose Down
DROP TABLE comment_likes;

ALTER TABLE comments
    DROP COLUMN like_count,
    DROP COLUMN reply_to;
