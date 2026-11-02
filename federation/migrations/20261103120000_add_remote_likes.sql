-- +goose Up
-- The card of the page a remote post links to, when its server tells one.
ALTER TABLE remote_posts
    ADD COLUMN link jsonb;

-- Who here like remote posts: their servers do not learn of it. The IDs
-- are UUIDv7, the cursor of the likers.
CREATE TABLE remote_post_likes (
    id         uuid        PRIMARY KEY,
    post_id    uuid        NOT NULL REFERENCES remote_posts (id) ON DELETE CASCADE,
    user_id    uuid        NOT NULL,
    created_at timestamptz NOT NULL,
    UNIQUE (post_id, user_id)
);

CREATE INDEX remote_post_likes_post_idx ON remote_post_likes (post_id, id);

-- +goose Down
DROP TABLE remote_post_likes;

ALTER TABLE remote_posts
    DROP COLUMN link;
