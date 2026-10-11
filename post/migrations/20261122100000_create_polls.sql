-- +goose Up
-- "Опрос" of openvk: a post may ask a poll, one at most, known by the ID
-- of its post and gone with it. A poll is content enough for a post.
ALTER TABLE posts
    ADD COLUMN polled boolean NOT NULL DEFAULT false,
    DROP CONSTRAINT posts_content_check,
    ADD CONSTRAINT posts_content_check
        CHECK (text <> '' OR cardinality(photo_ids) > 0 OR repost_of IS NOT NULL OR shared_photo_id IS NOT NULL OR polled);

CREATE TABLE polls (
    post_id     uuid        PRIMARY KEY REFERENCES posts (id) ON DELETE CASCADE,
    title       text        NOT NULL CHECK (title <> '' AND char_length(title) <= 128),
    anonymous   boolean     NOT NULL,
    multiple    boolean     NOT NULL,
    locked      boolean     NOT NULL,
    -- NULL: open for good.
    ends_at     timestamptz,
    -- How many people voted: with several choices fewer than the votes.
    voter_count integer     NOT NULL DEFAULT 0 CHECK (voter_count >= 0)
);

CREATE TABLE poll_options (
    post_id    uuid     NOT NULL REFERENCES polls (post_id) ON DELETE CASCADE,
    place      smallint NOT NULL CHECK (place BETWEEN 0 AND 9),
    name       text     NOT NULL CHECK (name <> '' AND char_length(name) <= 128),
    vote_count integer  NOT NULL DEFAULT 0 CHECK (vote_count >= 0),
    PRIMARY KEY (post_id, place)
);

CREATE TABLE poll_votes (
    post_id    uuid        NOT NULL,
    place      smallint    NOT NULL,
    user_id    uuid        NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (post_id, user_id, place),
    FOREIGN KEY (post_id, place) REFERENCES poll_options (post_id, place) ON DELETE CASCADE
);

-- Who chose an option, newest first.
CREATE INDEX poll_votes_option_idx ON poll_votes (post_id, place, created_at DESC, user_id DESC);
-- The votes of a user whose page is erased.
CREATE INDEX poll_votes_user_id_idx ON poll_votes (user_id);

-- +goose Down
DROP TABLE poll_votes;
DROP TABLE poll_options;
DROP TABLE polls;

-- Posts of a poll only cannot exist without it.
DELETE FROM posts
WHERE text = '' AND cardinality(photo_ids) = 0 AND repost_of IS NULL AND shared_photo_id IS NULL;

ALTER TABLE posts
    DROP CONSTRAINT posts_content_check,
    ADD CONSTRAINT posts_content_check
        CHECK (text <> '' OR cardinality(photo_ids) > 0 OR repost_of IS NOT NULL OR shared_photo_id IS NOT NULL),
    DROP COLUMN polled;
