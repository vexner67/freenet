-- +goose Up
-- One followed back one's follower, accepting the friend request
-- ('friend'): one per pair, as a follow.
ALTER TABLE notifications
    DROP CONSTRAINT notifications_kind_check,
    ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('like', 'comment', 'follow', 'wall_post', 'repost', 'comment_reply', 'comment_like', 'post_accepted', 'suggested_post', 'club_admin', 'topic_message', 'post_mention', 'comment_mention', 'topic_mention', 'photo_like', 'photo_comment', 'topic_message_like', 'photo_mention', 'gift', 'votes', 'invite', 'verified', 'club_verified', 'story_like', 'story_comment', 'prime', 'match', 'call', 'missed_call', 'friend'));

CREATE UNIQUE INDEX notifications_friend_key ON notifications (actor_id, recipient_id) WHERE kind = 'friend';

-- +goose Down
DELETE FROM notifications WHERE kind = 'friend';

DROP INDEX notifications_friend_key;

ALTER TABLE notifications
    DROP CONSTRAINT notifications_kind_check,
    ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('like', 'comment', 'follow', 'wall_post', 'repost', 'comment_reply', 'comment_like', 'post_accepted', 'suggested_post', 'club_admin', 'topic_message', 'post_mention', 'comment_mention', 'topic_mention', 'photo_like', 'photo_comment', 'topic_message_like', 'photo_mention', 'gift', 'votes', 'invite', 'verified', 'club_verified', 'story_like', 'story_comment', 'prime', 'match', 'call', 'missed_call'));
