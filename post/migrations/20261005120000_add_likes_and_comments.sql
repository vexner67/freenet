-- +goose Up
-- Counters live in the post, so walls and feeds are read without counting.
-- They change in the same statement as the like or comment.
ALTER TABLE posts
    ADD COLUMN like_count    integer NOT NULL DEFAULT 0 CHECK (like_count >= 0),
    ADD COLUMN comment_count integer NOT NULL DEFAULT 0 CHECK (comment_count >= 0);

-- user_id refers to the user service's database, so there is no foreign key.
-- The primary key makes a like idempotent and answers "do I like these
-- posts": post_id = ANY($posts) AND user_id = $1.
CREATE TABLE post_likes (
    post_id    uuid        NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    user_id    uuid        NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (post_id, user_id)
);

CREATE TABLE comments (
    id         uuid        PRIMARY KEY,
    post_id    uuid        NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    author_id  uuid        NOT NULL,
    text       text        NOT NULL CHECK (text <> ''),
    created_at timestamptz NOT NULL
);

-- Comments are read oldest first by keyset: post_id = $1 AND id > $cursor.
CREATE INDEX comments_post_id_idx ON comments (post_id, id);

-- +goose Down
DROP TABLE comments;
DROP TABLE post_likes;

ALTER TABLE posts
    DROP COLUMN comment_count,
    DROP COLUMN like_count;
