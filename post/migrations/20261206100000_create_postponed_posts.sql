-- +goose Up
-- Posts that wait for their time, VK's "отложенные записи" (openvk has
-- none), apart from posts as suggestions are: no wall, feed or count sees
-- them until publish_at, when the service makes each a new post. A post
-- waits for its author's own wall (wall_id = author_id) or speaks for a
-- community (from_wall). wall_id, author_id and mentioned_ids refer to the
-- club and user services' databases, so there are no foreign keys.
-- photo_ids are kept for good by the media service from the moment of
-- postponing.
CREATE TABLE postponed_posts (
    id            uuid        PRIMARY KEY,
    wall_id       uuid        NOT NULL,
    author_id     uuid        NOT NULL,
    text          text        NOT NULL CHECK (char_length(text) <= 60000),
    photo_ids     uuid[]      NOT NULL DEFAULT '{}' CHECK (cardinality(photo_ids) <= 10),
    from_wall     boolean     NOT NULL DEFAULT false,
    signed        boolean     NOT NULL DEFAULT false,
    nsfw          boolean     NOT NULL DEFAULT false,
    mentioned_ids uuid[]      NOT NULL DEFAULT '{}' CHECK (cardinality(mentioned_ids) <= 10),
    publish_at    timestamptz NOT NULL,
    created_at    timestamptz NOT NULL,
    CHECK (text <> '' OR cardinality(photo_ids) > 0),
    CHECK (from_wall = (wall_id <> author_id)),
    CHECK (from_wall OR NOT signed)
);

-- The posts that wait for a wall, the soonest first.
CREATE INDEX postponed_posts_wall_idx ON postponed_posts (wall_id, publish_at, id);
-- The posts whose time has come.
CREATE INDEX postponed_posts_due_idx ON postponed_posts (publish_at, id);
-- What a user postponed, erased with them.
CREATE INDEX postponed_posts_author_idx ON postponed_posts (author_id);

-- +goose Down
DROP TABLE postponed_posts;
