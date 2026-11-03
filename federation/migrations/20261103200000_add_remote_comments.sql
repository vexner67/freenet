-- +goose Up
-- What people here write under remote posts: their servers do not learn
-- of it. The IDs are UUIDv7, the cursor of the comments.
CREATE TABLE remote_post_comments (
    id         uuid        PRIMARY KEY,
    post_id    uuid        NOT NULL REFERENCES remote_posts (id) ON DELETE CASCADE,
    author_id  uuid        NOT NULL,
    text       text        NOT NULL CHECK (length(text) BETWEEN 1 AND 60000),
    created_at timestamptz NOT NULL
);

CREATE INDEX remote_post_comments_post_idx ON remote_post_comments (post_id, id);

-- +goose Down
DROP TABLE remote_post_comments;
