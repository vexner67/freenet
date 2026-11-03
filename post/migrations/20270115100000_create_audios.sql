-- +goose Up
-- "Аудиозаписи" of openvk: songs people upload, their bytes in the media
-- service under the same ID and owner, and tracks of SoundCloud and
-- YouTube, one per address, shared by all who add them. owner_id refers
-- to the user service's database, so there is no foreign key.
CREATE TABLE audios (
    id         uuid        PRIMARY KEY,
    owner_id   uuid        NOT NULL,
    kind       text        NOT NULL CHECK (kind IN ('file', 'soundcloud', 'youtube')),
    link       text        NOT NULL CHECK ((kind = 'file') = (link = '') AND char_length(link) <= 512),
    cover      text        NOT NULL CHECK (char_length(cover) <= 512),
    artist     text        NOT NULL CHECK (char_length(artist) <= 128),
    title      text        NOT NULL CHECK (char_length(title) <= 128),
    duration   integer     NOT NULL CHECK (duration BETWEEN 0 AND 10800),
    created_at timestamptz NOT NULL
);

-- A track is one song whoever adds it.
CREATE UNIQUE INDEX audios_link_idx ON audios (link) WHERE link <> '';
-- The uploads of a person, for the deletion of the page.
CREATE INDEX audios_owner_idx ON audios (owner_id) WHERE kind = 'file';

-- Playlists of openvk: lists of songs a person makes.
CREATE TABLE playlists (
    id         uuid        PRIMARY KEY,
    owner_id   uuid        NOT NULL,
    title      text        NOT NULL CHECK (title <> '' AND char_length(title) <= 128),
    created_at timestamptz NOT NULL
);

CREATE INDEX playlists_owner_idx ON playlists (owner_id, id);

-- The songs of a list: the library of a person under their ID, or a
-- playlist under its. id is the place a song was added at, the last on
-- top.
CREATE TABLE audio_entries (
    id       uuid PRIMARY KEY,
    list_id  uuid NOT NULL,
    audio_id uuid NOT NULL REFERENCES audios (id) ON DELETE CASCADE,
    UNIQUE (list_id, audio_id)
);

CREATE INDEX audio_entries_list_idx ON audio_entries (list_id, id);
CREATE INDEX audio_entries_audio_idx ON audio_entries (audio_id);

-- What a page says of music: the song its owner listens to now and since
-- when ("слушает"), and the song of the page.
CREATE TABLE music_profiles (
    user_id       uuid PRIMARY KEY,
    listening_id  uuid REFERENCES audios (id) ON DELETE SET NULL,
    listening_at  timestamptz,
    page_audio_id uuid REFERENCES audios (id) ON DELETE SET NULL
);

-- A post attaches songs by ID, without a foreign key: a deleted song
-- leaves the post. Songs are content enough for a post.
ALTER TABLE posts
    ADD COLUMN audio_ids uuid[] NOT NULL DEFAULT '{}' CHECK (cardinality(audio_ids) <= 10),
    DROP CONSTRAINT posts_content_check,
    ADD CONSTRAINT posts_content_check
        CHECK (text <> '' OR cardinality(photo_ids) > 0 OR repost_of IS NOT NULL OR shared_photo_id IS NOT NULL OR polled
            OR cardinality(document_ids) > 0 OR cardinality(audio_ids) > 0);

-- +goose Down
DELETE FROM posts
WHERE text = '' AND cardinality(photo_ids) = 0 AND repost_of IS NULL AND shared_photo_id IS NULL AND NOT polled
    AND cardinality(document_ids) = 0;

ALTER TABLE posts
    DROP CONSTRAINT posts_content_check,
    ADD CONSTRAINT posts_content_check
        CHECK (text <> '' OR cardinality(photo_ids) > 0 OR repost_of IS NOT NULL OR shared_photo_id IS NOT NULL OR polled
            OR cardinality(document_ids) > 0),
    DROP COLUMN audio_ids;

DROP TABLE music_profiles;
DROP TABLE audio_entries;
DROP TABLE playlists;
DROP TABLE audios;
