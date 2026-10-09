-- +goose Up
-- Photo albums of people and communities ("Альбомы" of openvk). owner_id
-- is a user or a club, as wall_id of a post, and author_id who uploaded
-- the photo: they refer to the user and club services' databases, so
-- there are no foreign keys to them. An album counts its photos and keeps
-- the time of the last change (updated_at) in the same statements that
-- add and remove them.
CREATE TABLE albums (
    id          uuid        PRIMARY KEY,
    owner_id    uuid        NOT NULL,
    name        text        NOT NULL CHECK (name <> '' AND char_length(name) <= 36),
    description text        NOT NULL DEFAULT '' CHECK (char_length(description) <= 255),
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL,
    photo_count integer     NOT NULL DEFAULT 0 CHECK (photo_count >= 0)
);

-- The albums of an owner, newest first.
CREATE INDEX albums_owner_idx ON albums (owner_id, id DESC);

-- id is the photo's in the media service, which keeps the image: a photo
-- is in one album at most.
CREATE TABLE album_photos (
    id          uuid        PRIMARY KEY,
    album_id    uuid        NOT NULL REFERENCES albums (id) ON DELETE CASCADE,
    author_id   uuid        NOT NULL,
    description text        NOT NULL DEFAULT '' CHECK (char_length(description) <= 255),
    created_at  timestamptz NOT NULL
);

CREATE INDEX album_photos_album_idx ON album_photos (album_id, id);
CREATE INDEX album_photos_author_idx ON album_photos (author_id);

-- +goose Down
DROP TABLE album_photos;
DROP TABLE albums;
