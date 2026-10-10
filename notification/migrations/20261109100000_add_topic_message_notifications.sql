-- +goose Up
-- A message in a topic: someone wrote in the topic the recipient started;
-- post_id is the topic and comment_id the message. One per message; the
-- message deleted takes it (ForgetComment), the topic all of them
-- (ForgetPost).
ALTER TABLE notifications
    DROP CONSTRAINT notifications_kind_check,
    ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('like', 'comment', 'follow', 'wall_post', 'repost', 'comment_reply', 'comment_like', 'post_accepted', 'suggested_post', 'club_admin', 'topic_message')),
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
            WHEN 'topic_message'  THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            ELSE post_id IS NULL AND comment_id IS NULL
        END
    );

CREATE UNIQUE INDEX notifications_topic_message_key ON notifications (comment_id) WHERE kind = 'topic_message';

-- +goose Down
DELETE FROM notifications WHERE kind = 'topic_message';

DROP INDEX notifications_topic_message_key;

ALTER TABLE notifications
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
    ),
    DROP CONSTRAINT notifications_kind_check,
    ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('like', 'comment', 'follow', 'wall_post', 'repost', 'comment_reply', 'comment_like', 'post_accepted', 'suggested_post', 'club_admin'));
