-- +goose Up
-- Articles: long reads written in the editor, as "Статьи" of VK. The text
-- is blocks of spans (jsonb), checked by the post service; author_id
-- refers to the user service's database, so there is no foreign key.
CREATE TABLE articles (
    id         uuid        PRIMARY KEY,
    author_id  uuid        NOT NULL,
    title      text        NOT NULL CHECK (title <> '' AND char_length(title) <= 256),
    cover_id   uuid,
    blocks     jsonb       NOT NULL CHECK (jsonb_typeof(blocks) = 'array'),
    created_at timestamptz NOT NULL,
    edited_at  timestamptz
);

-- The articles of a person, newest first.
CREATE INDEX articles_author_idx ON articles (author_id, id);

-- +goose Down
DROP TABLE articles;
