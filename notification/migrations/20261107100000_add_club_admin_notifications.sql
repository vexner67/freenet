-- +goose Up
-- An appointment: the recipient was made an admin of actor_id, a
-- community. One per community and admin; the dismissal takes it back.
ALTER TABLE notifications
    DROP CONSTRAINT notifications_kind_check,
    ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('like', 'comment', 'follow', 'wall_post', 'repost', 'comment_reply', 'comment_like', 'post_accepted', 'suggested_post', 'club_admin'));

CREATE UNIQUE INDEX notifications_club_admin_key ON notifications (actor_id, recipient_id) WHERE kind = 'club_admin';

-- +goose Down
DELETE FROM notifications WHERE kind = 'club_admin';

DROP INDEX notifications_club_admin_key;

ALTER TABLE notifications
    DROP CONSTRAINT notifications_kind_check,
    ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('like', 'comment', 'follow', 'wall_post', 'repost', 'comment_reply', 'comment_like', 'post_accepted', 'suggested_post'));
