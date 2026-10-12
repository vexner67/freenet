-- +goose Up
-- A gift ("Подарки"): the actor sent the recipient the gift post_id, which
-- the user service keeps. One per sent gift; a deleted gift takes it back.
ALTER TABLE notifications
    DROP CONSTRAINT notifications_kind_check,
    ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('like', 'comment', 'follow', 'wall_post', 'repost', 'comment_reply', 'comment_like', 'post_accepted', 'suggested_post', 'club_admin', 'topic_message', 'post_mention', 'comment_mention', 'topic_mention', 'photo_like', 'photo_comment', 'topic_message_like', 'event_reminder', 'photo_mention', 'note_comment', 'gift')),
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
            WHEN 'post_mention'    THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'comment_mention' THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'topic_mention'   THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'photo_like'         THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'photo_comment'      THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'topic_message_like' THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'photo_mention'      THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'note_comment'       THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'gift'               THEN post_id IS NOT NULL AND comment_id IS NULL
            ELSE post_id IS NULL AND comment_id IS NULL
        END
    );

CREATE UNIQUE INDEX notifications_gift_key ON notifications (post_id) WHERE kind = 'gift';

-- +goose Down
DELETE FROM notifications WHERE kind = 'gift';

DROP INDEX notifications_gift_key;

ALTER TABLE notifications
    DROP CONSTRAINT notifications_kind_check,
    ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('like', 'comment', 'follow', 'wall_post', 'repost', 'comment_reply', 'comment_like', 'post_accepted', 'suggested_post', 'club_admin', 'topic_message', 'post_mention', 'comment_mention', 'topic_mention', 'photo_like', 'photo_comment', 'topic_message_like', 'event_reminder', 'photo_mention', 'note_comment')),
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
            WHEN 'post_mention'    THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'comment_mention' THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'topic_mention'   THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'photo_like'         THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'photo_comment'      THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'topic_message_like' THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'photo_mention'      THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'note_comment'       THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            ELSE post_id IS NULL AND comment_id IS NULL
        END
    );
