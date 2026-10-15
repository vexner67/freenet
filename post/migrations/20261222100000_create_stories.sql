-- +goose Up
-- Stories, VK's "истории" (openvk has none): a photo with a few words a
-- person shows for a day to those who read them. The service deletes them
-- with their photos once the day is over; the queries also leave out those
-- older than a day. author_id refers to the user service's database and
-- photo_id to the media service's, so there are no foreign keys; the
-- photo is kept for good by media from the moment the story is told.
CREATE TABLE stories (
    id         uuid        PRIMARY KEY,
    author_id  uuid        NOT NULL,
    photo_id   uuid        NOT NULL,
    text       text        NOT NULL CHECK (char_length(text) <= 200),
    created_at timestamptz NOT NULL
);

-- The stories of a person, the oldest first; and those whose day is over.
CREATE INDEX stories_author_idx ON stories (author_id, created_at, id);
CREATE INDEX stories_created_idx ON stories (created_at, id);

-- Who viewed which story, once each.
CREATE TABLE story_views (
    story_id  uuid        NOT NULL REFERENCES stories (id) ON DELETE CASCADE,
    viewer_id uuid        NOT NULL,
    viewed_at timestamptz NOT NULL,
    PRIMARY KEY (story_id, viewer_id)
);

-- The viewers of a story, the latest first.
CREATE INDEX story_views_latest_idx ON story_views (story_id, viewed_at DESC, viewer_id DESC);
-- What a viewer viewed, erased with them.
CREATE INDEX story_views_viewer_idx ON story_views (viewer_id);

-- +goose Down
DROP TABLE story_views;
DROP TABLE stories;
