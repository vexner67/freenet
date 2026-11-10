-- +goose Up
ALTER TABLE notifications
    DROP CONSTRAINT notifications_kind_check,
    ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('like', 'comment', 'follow', 'wall_post', 'repost', 'comment_reply', 'comment_like', 'post_accepted', 'suggested_post', 'club_admin', 'topic_message', 'post_mention', 'comment_mention', 'topic_mention', 'topic_message_like', 'gift', 'votes', 'invite', 'verified', 'club_verified', 'story_like', 'story_comment', 'prime', 'match', 'call', 'missed_call', 'friend', 'room_invite')),
    DROP CONSTRAINT notifications_target_check,
    ADD CONSTRAINT notifications_target_check CHECK (
        CASE kind
            WHEN 'like'               THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'comment'            THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'wall_post'          THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'repost'             THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'comment_reply'      THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'comment_like'       THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'post_accepted'      THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'suggested_post'     THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'topic_message'      THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'post_mention'       THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'comment_mention'    THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'topic_mention'      THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'topic_message_like' THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'gift'               THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'votes'              THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'story_like'         THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'story_comment'      THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'room_invite'        THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'prime'              THEN post_id IS NOT NULL AND comment_id IS NULL
            ELSE post_id IS NULL AND comment_id IS NULL
        END
    );
CREATE UNIQUE INDEX notifications_room_invite_key ON notifications (actor_id,recipient_id,post_id) WHERE kind='room_invite';

-- +goose Down
DELETE FROM notifications WHERE kind='room_invite';
DROP INDEX notifications_room_invite_key;
ALTER TABLE notifications
    DROP CONSTRAINT notifications_kind_check,
    ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('like', 'comment', 'follow', 'wall_post', 'repost', 'comment_reply', 'comment_like', 'post_accepted', 'suggested_post', 'club_admin', 'topic_message', 'post_mention', 'comment_mention', 'topic_mention', 'topic_message_like', 'gift', 'votes', 'invite', 'verified', 'club_verified', 'story_like', 'story_comment', 'prime', 'match', 'call', 'missed_call', 'friend')),
    DROP CONSTRAINT notifications_target_check,
    ADD CONSTRAINT notifications_target_check CHECK (
        CASE kind
            WHEN 'like'               THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'comment'            THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'wall_post'          THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'repost'             THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'comment_reply'      THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'comment_like'       THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'post_accepted'      THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'suggested_post'     THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'topic_message'      THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'post_mention'       THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'comment_mention'    THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'topic_mention'      THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'topic_message_like' THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'gift'               THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'votes'              THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'story_like'         THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'story_comment'      THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'prime'              THEN post_id IS NOT NULL AND comment_id IS NULL
            ELSE post_id IS NULL AND comment_id IS NULL
        END
    );
