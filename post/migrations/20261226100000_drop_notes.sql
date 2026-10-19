-- +goose Up
-- Notes ("Заметки") are no more (the owner of the site decided so): they
-- go with their comments. Notes had no photos, and notification drops the
-- notifications about their comments itself.
DROP FUNCTION hidden_note_comments(uuid);
DROP TABLE note_comments;
DROP TABLE notes;

-- +goose Down
-- The tables are back empty; the deleted notes do not come back.
CREATE TABLE notes (
    id            uuid        PRIMARY KEY,
    author_id     uuid        NOT NULL,
    title         text        NOT NULL CHECK (title <> '' AND char_length(title) <= 256),
    text          text        NOT NULL CHECK (text <> '' AND char_length(text) <= 60000),
    comment_count integer     NOT NULL DEFAULT 0 CHECK (comment_count >= 0),
    created_at    timestamptz NOT NULL,
    edited_at     timestamptz
);

CREATE INDEX notes_author_idx ON notes (author_id, id DESC);

CREATE TABLE note_comments (
    id         uuid        PRIMARY KEY,
    note_id    uuid        NOT NULL REFERENCES notes (id) ON DELETE CASCADE,
    author_id  uuid        NOT NULL,
    text       text        NOT NULL CHECK (text <> '' AND char_length(text) <= 60000),
    created_at timestamptz NOT NULL
);

CREATE INDEX note_comments_note_idx ON note_comments (note_id, id);
CREATE INDEX note_comments_author_idx ON note_comments (author_id);

-- +goose StatementBegin
CREATE FUNCTION hidden_note_comments(note uuid) RETURNS integer
LANGUAGE sql STABLE AS $$
    SELECT count(*)::integer
    FROM hidden_users h
    JOIN note_comments c ON c.author_id = h.user_id
    WHERE c.note_id = note
$$;
-- +goose StatementEnd
