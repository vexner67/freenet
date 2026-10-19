-- +goose Up
-- The checkmark ("Подтверждённая страница" of openvk): an administrator of
-- the site, the actor, confirmed the recipient's page (verified), or the
-- community the recipient owns, the actor, was confirmed (club_verified).
-- Nothing but the two is named; one per page and one per community,
-- however often it is given, and taken back with the checkmark.
ALTER TABLE notifications
    DROP CONSTRAINT notifications_kind_check,
    ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('like', 'comment', 'follow', 'wall_post', 'repost', 'comment_reply', 'comment_like', 'post_accepted', 'suggested_post', 'club_admin', 'topic_message', 'post_mention', 'comment_mention', 'topic_mention', 'photo_like', 'photo_comment', 'topic_message_like', 'photo_mention', 'note_comment', 'gift', 'votes', 'invite', 'verified', 'club_verified')),
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
            WHEN 'votes'              THEN post_id IS NOT NULL AND comment_id IS NULL
            ELSE post_id IS NULL AND comment_id IS NULL
        END
    );

CREATE UNIQUE INDEX notifications_verified_key ON notifications (recipient_id) WHERE kind = 'verified';
CREATE UNIQUE INDEX notifications_club_verified_key ON notifications (actor_id) WHERE kind = 'club_verified';

-- +goose Down
DELETE FROM notifications WHERE kind IN ('verified', 'club_verified');

DROP INDEX notifications_verified_key;
DROP INDEX notifications_club_verified_key;

ALTER TABLE notifications
    DROP CONSTRAINT notifications_kind_check,
    ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('like', 'comment', 'follow', 'wall_post', 'repost', 'comment_reply', 'comment_like', 'post_accepted', 'suggested_post', 'club_admin', 'topic_message', 'post_mention', 'comment_mention', 'topic_mention', 'photo_like', 'photo_comment', 'topic_message_like', 'photo_mention', 'note_comment', 'gift', 'votes', 'invite')),
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
            WHEN 'votes'              THEN post_id IS NOT NULL AND comment_id IS NULL
            ELSE post_id IS NULL AND comment_id IS NULL
        END
    );
