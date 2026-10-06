-- +goose Up
-- A wall post: someone wrote on the recipient's wall. One per post.
-- notifications_check1 is the unnamed target check of the first migration
-- (notifications_check is actor_id <> recipient_id); it gets a name now.
ALTER TABLE notifications
    DROP CONSTRAINT notifications_kind_check,
    ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('like', 'comment', 'follow', 'wall_post')),
    DROP CONSTRAINT notifications_check1,
    ADD CONSTRAINT notifications_target_check CHECK (
        CASE kind
            WHEN 'like'      THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'comment'   THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'wall_post' THEN post_id IS NOT NULL AND comment_id IS NULL
            ELSE post_id IS NULL AND comment_id IS NULL
        END
    );

CREATE UNIQUE INDEX notifications_wall_post_key ON notifications (post_id) WHERE kind = 'wall_post';

-- +goose Down
DELETE FROM notifications WHERE kind = 'wall_post';

DROP INDEX notifications_wall_post_key;

ALTER TABLE notifications
    DROP CONSTRAINT notifications_target_check,
    ADD CONSTRAINT notifications_check1 CHECK (
        CASE kind
            WHEN 'like'    THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'comment' THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            ELSE post_id IS NULL AND comment_id IS NULL
        END
    ),
    DROP CONSTRAINT notifications_kind_check,
    ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('like', 'comment', 'follow'));
