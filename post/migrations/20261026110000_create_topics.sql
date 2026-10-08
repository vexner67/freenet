-- +goose Up
-- The discussions of communities ("Обсуждения" of openvk): topics and the
-- messages in them. club_id and author_id refer to the club and user
-- services' databases, so there are no foreign keys to them. A topic
-- counts its messages and keeps the time of the newest (updated_at) in
-- the same statements that add and remove them.
CREATE TABLE topics (
    id            uuid        PRIMARY KEY,
    club_id       uuid        NOT NULL,
    author_id     uuid        NOT NULL,
    title         text        NOT NULL CHECK (title <> '' AND char_length(title) <= 127),
    from_wall     boolean     NOT NULL DEFAULT false,
    closed        boolean     NOT NULL DEFAULT false,
    pinned        boolean     NOT NULL DEFAULT false,
    -- Only the admins write in it: only a topic of the community.
    restricted    boolean     NOT NULL DEFAULT false CHECK (NOT restricted OR from_wall),
    created_at    timestamptz NOT NULL,
    updated_at    timestamptz NOT NULL,
    message_count integer     NOT NULL DEFAULT 0 CHECK (message_count >= 0)
);

-- The list of a club: pinned first, then the most recently active.
CREATE INDEX topics_club_idx ON topics (club_id, pinned DESC, updated_at DESC, id DESC);
CREATE INDEX topics_author_idx ON topics (author_id);

-- photo_ids are kept by the media service, as those of comments.
CREATE TABLE topic_messages (
    id         uuid        PRIMARY KEY,
    topic_id   uuid        NOT NULL REFERENCES topics (id) ON DELETE CASCADE,
    author_id  uuid        NOT NULL,
    text       text        NOT NULL CHECK (char_length(text) <= 1000),
    photo_ids  uuid[]      NOT NULL DEFAULT '{}' CHECK (cardinality(photo_ids) <= 10),
    from_wall  boolean     NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL,
    CHECK (text <> '' OR cardinality(photo_ids) > 0)
);

CREATE INDEX topic_messages_topic_idx ON topic_messages (topic_id, id);
CREATE INDEX topic_messages_author_idx ON topic_messages (author_id);

-- +goose Down
DROP TABLE topic_messages;
DROP TABLE topics;
