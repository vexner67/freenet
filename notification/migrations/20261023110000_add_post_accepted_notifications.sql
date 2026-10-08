-- +goose Up
-- An accepted post: a community published the post the recipient
-- suggested for its wall; actor_id is the community, post_id the
-- published post. One per post; a deleted post takes it (ForgetPost).
ALTER TABLE notifications
    DROP CONSTRAINT notifications_kind_check,
    ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('like', 'comment', 'follow', 'wall_post', 'repost', 'comment_reply', 'comment_like', 'post_accepted')),
    DROP CONSTRAINT notifications_target_check,
    ADD CONSTRAINT notifications_target_check CHECK (
        CASE kind
            WHEN 'like'          THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'comment'       THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'wall_post'     THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'repost'        THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'comment_reply' THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'comment_like'  THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'post_accepted' THEN post_id IS NOT NULL AND comment_id IS NULL
            ELSE post_id IS NULL AND comment_id IS NULL
        END
    );

CREATE UNIQUE INDEX notifications_post_accepted_key ON notifications (post_id) WHERE kind = 'post_accepted';

-- +goose Down
DELETE FROM notifications WHERE kind = 'post_accepted';

DROP INDEX notifications_post_accepted_key;

ALTER TABLE notifications
    DROP CONSTRAINT notifications_target_check,
    ADD CONSTRAINT notifications_target_check CHECK (
        CASE kind
            WHEN 'like'          THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'comment'       THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'wall_post'     THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'repost'        THEN post_id IS NOT NULL AND comment_id IS NULL
            WHEN 'comment_reply' THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            WHEN 'comment_like'  THEN post_id IS NOT NULL AND comment_id IS NOT NULL
            ELSE post_id IS NULL AND comment_id IS NULL
        END
    ),
    DROP CONSTRAINT notifications_kind_check,
    ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('like', 'comment', 'follow', 'wall_post', 'repost', 'comment_reply', 'comment_like'));
