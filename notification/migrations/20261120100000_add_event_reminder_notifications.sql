-- +goose Up
-- A reminder: actor_id, an event the recipient comes to, starts soon
-- ("В ближайшее время состоится событие" of openvk). One per event and
-- member; an event moved to another day takes it back and reminds again.
ALTER TABLE notifications
    DROP CONSTRAINT notifications_kind_check,
    ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('like', 'comment', 'follow', 'wall_post', 'repost', 'comment_reply', 'comment_like', 'post_accepted', 'suggested_post', 'club_admin', 'topic_message', 'post_mention', 'comment_mention', 'topic_mention', 'photo_like', 'photo_comment', 'topic_message_like', 'event_reminder'));

CREATE UNIQUE INDEX notifications_event_reminder_key ON notifications (actor_id, recipient_id) WHERE kind = 'event_reminder';

-- +goose Down
DELETE FROM notifications WHERE kind = 'event_reminder';

DROP INDEX notifications_event_reminder_key;

ALTER TABLE notifications
    DROP CONSTRAINT notifications_kind_check,
    ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('like', 'comment', 'follow', 'wall_post', 'repost', 'comment_reply', 'comment_like', 'post_accepted', 'suggested_post', 'club_admin', 'topic_message', 'post_mention', 'comment_mention', 'topic_mention', 'photo_like', 'photo_comment', 'topic_message_like'));
