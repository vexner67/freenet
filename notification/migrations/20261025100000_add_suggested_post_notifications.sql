-- +goose Up
-- A suggested post: someone suggested a post for the wall of actor_id, a
-- community the recipient runs; post_id is the suggestion, not a post.
-- One per suggestion and admin; the suggestion published, declined or
-- withdrawn takes them (ForgetPost).
ALTER TABLE notifications
    DROP CONSTRAINT notifications_kind_check,
    ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('like', 'comment', 'follow', 'wall_post', 'repost', 'comment_reply', 'comment_like', 'post_accepted', 'suggested_post')),
    DROP CONSTRAINT notifications_target_check,
    ADD CONSTRAINT notifications_target_check CHECK (
        CASE kind
            WHEN 'like'           THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'comment'        THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'wall_post'      THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'repost'         THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'comment_reply'  THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'comment_like'   THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'post_accepted'  THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'suggested_post' THEN post_id IS NOT NULL AND comment_id IS NULL
            ELSE post_id IS NULL AND comment_id IS NULL
        END
    );

CREATE UNIQUE INDEX notifications_suggested_post_key ON notifications (post_id, recipient_id) WHERE kind = 'suggested_post';

-- +goose Down
DELETE FROM notifications WHERE kind = 'suggested_post';

DROP INDEX notifications_suggested_post_key;

ALTER TABLE notifications
    DROP CONSTRAINT notifications_target_check,
    ADD CONSTRAINT notifications_target_check CHECK (
        CASE kind
            WHEN 'like'          THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'comment'       THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'wall_post'     THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'repost'        THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'comment_reply' THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'comment_like'  THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'post_accepted' THEN post_id IS NOT NULL AND comment_id IS NULL
            ELSE post_id IS NULL AND comment_id IS NULL
        END
    ),
    DROP CONSTRAINT notifications_kind_check,
    ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('like', 'comment', 'follow', 'wall_post', 'repost', 'comment_reply', 'comment_like', 'post_accepted'));
