-- +goose Up
-- recipient_id, actor_id, post_id and comment_id refer to other services'
-- databases, so there are no foreign keys: post and user take notifications
-- back themselves when a like, comment, follow or post is gone.
CREATE TABLE notifications (
    id           uuid        PRIMARY KEY,
    kind         text        NOT NULL CHECK (kind IN ('like', 'comment', 'follow')),
    recipient_id uuid        NOT NULL,
    actor_id     uuid        NOT NULL CHECK (actor_id <> recipient_id),
    post_id      uuid,
    comment_id   uuid,
    created_at   timestamptz NOT NULL,
    CHECK (
        CASE kind
            WHEN 'like'    THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'comment' THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            ELSE post_id IS NULL AND comment_id IS NULL
        END
    )
);

-- A user's notifications are read newest first by keyset and counted
-- above the read mark: recipient_id = $1 AND id > $mark.
CREATE INDEX notifications_recipient_id_idx ON notifications (recipient_id, id DESC);

-- One notification per like, comment and follow: a repeated event is not
-- notified twice, and undoing it deletes exactly this row.
CREATE UNIQUE INDEX notifications_like_key ON notifications (actor_id, post_id) WHERE kind = 'like';
CREATE UNIQUE INDEX notifications_comment_key ON notifications (comment_id) WHERE kind = 'comment';
CREATE UNIQUE INDEX notifications_follow_key ON notifications (actor_id, recipient_id) WHERE kind = 'follow';

-- A deleted post takes its notifications with it.
CREATE INDEX notifications_post_id_idx ON notifications (post_id) WHERE post_id IS NOT NULL;

-- The newest notification each user has read; newer ones are unread.
CREATE TABLE notification_reads (
    user_id    uuid PRIMARY KEY,
    read_up_to uuid NOT NULL
);

-- +goose Down
DROP TABLE notification_reads;
DROP TABLE notifications;
