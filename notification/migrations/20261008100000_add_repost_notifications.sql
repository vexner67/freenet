-- +goose Up
-- A repost: someone shared the recipient's post. post_id is the repost,
-- so deleting the repost (ForgetPost) takes the notification away. One
-- per repost.
ALTER TABLE notifications
    DROP CONSTRAINT notifications_kind_check,
    ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('like', 'comment', 'follow', 'wall_post', 'repost')),
    DROP CONSTRAINT notifications_target_check,
    ADD CONSTRAINT notifications_target_check CHECK (
        CASE kind
            WHEN 'like'      THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'comment'   THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'wall_post' THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'repost'    THEN post_id IS NOT NULL AND comment_id IS NULL
            ELSE post_id IS NULL AND comment_id IS NULL
        END
    );

CREATE UNIQUE INDEX notifications_repost_key ON notifications (post_id) WHERE kind = 'repost';

-- +goose Down
DELETE FROM notifications WHERE kind = 'repost';

DROP INDEX notifications_repost_key;

ALTER TABLE notifications
    DROP CONSTRAINT notifications_target_check,
    ADD CONSTRAINT notifications_target_check CHECK (
        CASE kind
            WHEN 'like'      THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'comment'   THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'wall_post' THEN post_id IS NOT NULL AND comment_id IS NULL
            ELSE post_id IS NULL AND comment_id IS NULL
        END
    ),
    DROP CONSTRAINT notifications_kind_check,
    ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('like', 'comment', 'follow', 'wall_post'));
