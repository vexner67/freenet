-- +goose Up
-- "Заметки" of openvk: a person's notes, a title and a text kept apart
-- from the wall, newest first by the UUIDv7 ID, and the comments under
-- them, text only, counted in the note by the statement that adds or
-- deletes one.
CREATE TABLE notes (
    id            uuid        PRIMARY KEY,
    author_id     uuid        NOT NULL,
    title         text        NOT NULL CHECK (title <> '' AND char_length(title) <= 256),
    text          text        NOT NULL CHECK (text <> '' AND char_length(text) <= 60000),
    comment_count integer     NOT NULL DEFAULT 0 CHECK (comment_count >= 0),
    created_at    timestamptz NOT NULL,
    -- NULL: never changed.
    edited_at     timestamptz
);

-- The notes of a person, newest first.
CREATE INDEX notes_author_idx ON notes (author_id, id DESC);

CREATE TABLE note_comments (
    id         uuid        PRIMARY KEY,
    note_id    uuid        NOT NULL REFERENCES notes (id) ON DELETE CASCADE,
    author_id  uuid        NOT NULL,
    text       text        NOT NULL CHECK (text <> '' AND char_length(text) <= 60000),
    created_at timestamptz NOT NULL
);

-- The comments of a note, oldest first.
CREATE INDEX note_comments_note_idx ON note_comments (note_id, id);
-- The comments a person left, to erase them with the page.
CREATE INDEX note_comments_author_idx ON note_comments (author_id);

-- +goose Down
DROP TABLE note_comments;
DROP TABLE notes;
