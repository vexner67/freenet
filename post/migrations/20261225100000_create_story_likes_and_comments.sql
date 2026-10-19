-- +goose Up
-- Stories are liked and commented, as on VK: the likes and comments go
-- with their story (ON DELETE CASCADE), a day after it is told at most.
-- Counted when read, without deleted pages: a story has few of either.
CREATE TABLE story_likes (
    story_id   uuid        NOT NULL REFERENCES stories (id) ON DELETE CASCADE,
    user_id    uuid        NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (story_id, user_id)
);

-- What a person liked, erased with them.
CREATE INDEX story_likes_user_idx ON story_likes (user_id);

CREATE TABLE story_comments (
    id         uuid        PRIMARY KEY,
    story_id   uuid        NOT NULL REFERENCES stories (id) ON DELETE CASCADE,
    author_id  uuid        NOT NULL,
    text       text        NOT NULL CHECK (char_length(text) BETWEEN 1 AND 1000),
    created_at timestamptz NOT NULL
);

-- The comments of a story, the oldest first; those of a person, erased
-- with them.
CREATE INDEX story_comments_story_idx ON story_comments (story_id, id);
CREATE INDEX story_comments_author_idx ON story_comments (author_id);

-- +goose Down
DROP TABLE story_comments;
DROP TABLE story_likes;
