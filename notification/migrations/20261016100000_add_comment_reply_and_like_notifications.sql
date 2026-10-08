-- +goose Up
-- A reply: someone answered the recipient's comment; comment_id is the
-- answer. A comment like: someone liked the recipient's comment
-- comment_id. One of each per reply and per like; a deleted comment takes
-- every notification of it (ForgetComment).
ALTER TABLE notifications
    DROP CONSTRAINT notifications_kind_check,
    ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('like', 'comment', 'follow', 'wall_post', 'repost', 'comment_reply', 'comment_like')),
    DROP CONSTRAINT notifications_target_check,
    ADD CONSTRAINT notifications_target_check CHECK (
        CASE kind
            WHEN 'like'          THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'comment'       THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'wall_post'     THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'repost'        THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'comment_reply' THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'comment_like'  THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            ELSE post_id IS NULL AND comment_id IS NULL
        END
    );

CREATE UNIQUE INDEX notifications_comment_reply_key ON notifications (comment_id) WHERE kind = 'comment_reply';
CREATE UNIQUE INDEX notifications_comment_like_key ON notifications (actor_id, comment_id) WHERE kind = 'comment_like';

-- ForgetComment.
CREATE INDEX notifications_comment_id_idx ON notifications (comment_id) WHERE comment_id IS NOT NULL;

-- +goose Down
DELETE FROM notifications WHERE kind IN ('comment_reply', 'comment_like');

DROP INDEX notifications_comment_id_idx;
DROP INDEX notifications_comment_like_key;
DROP INDEX notifications_comment_reply_key;

ALTER TABLE notifications
    DROP CONSTRAINT notifications_target_check,
    ADD CONSTRAINT notifications_target_check CHECK (
        CASE kind
            WHEN 'like'      THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'comment'   THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'wall_post' THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'repost'    THEN post_id IS NOT NULL AND comment_id IS NULL
            ELSE post_id IS NULL AND comment_id IS NULL
        END
    ),
    DROP CONSTRAINT notifications_kind_check,
    ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('like', 'comment', 'follow', 'wall_post', 'repost'));
