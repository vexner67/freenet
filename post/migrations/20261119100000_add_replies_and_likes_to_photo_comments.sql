-- +goose Up
-- Comments of photos answer one another and have hearts, as comments of
-- posts do: a deleted comment leaves its replies, which then answer
-- nothing; hearts are one per user, counted in like_count by the same
-- statement, deleted with the comment.
ALTER TABLE photo_comments
    ADD COLUMN reply_to   uuid REFERENCES photo_comments (id) ON DELETE SET NULL,
    ADD COLUMN like_count integer NOT NULL DEFAULT 0 CHECK (like_count >= 0);

-- ON DELETE SET NULL finds the replies of a deleted comment by it.
CREATE INDEX photo_comments_reply_to_idx ON photo_comments (reply_to) WHERE reply_to IS NOT NULL;

CREATE TABLE photo_comment_likes (
    comment_id uuid        NOT NULL REFERENCES photo_comments (id) ON DELETE CASCADE,
    user_id    uuid        NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (comment_id, user_id)
);

-- Who liked a comment of a photo or a message of a topic, newest first,
-- by keyset on (created_at, user_id): "Понравилось N людям".
CREATE INDEX photo_comment_likes_likers_idx ON photo_comment_likes (comment_id, created_at DESC, user_id DESC);
CREATE INDEX topic_message_likes_likers_idx ON topic_message_likes (message_id, created_at DESC, user_id DESC);
-- Erasing a user finds their likes by it.
CREATE INDEX photo_comment_likes_user_id_idx ON photo_comment_likes (user_id);

-- +goose Down
DROP INDEX topic_message_likes_likers_idx;
DROP TABLE photo_comment_likes;

ALTER TABLE photo_comments
    DROP COLUMN like_count,
    DROP COLUMN reply_to;
