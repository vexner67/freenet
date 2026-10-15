-- +goose Up
-- "Мои Документы" of openvk: files a person keeps, their bytes in the
-- media service under the same ID and owner. owner_id refers to the user
-- service's database, so there is no foreign key.
CREATE TABLE documents (
    id         uuid        PRIMARY KEY,
    owner_id   uuid        NOT NULL,
    name       text        NOT NULL CHECK (name <> '' AND char_length(name) <= 128),
    ext        text        NOT NULL CHECK (ext ~ '^[a-z0-9]{1,8}$'),
    size       bigint      NOT NULL CHECK (size > 0),
    created_at timestamptz NOT NULL
);

-- The documents of a person, newest first.
CREATE INDEX documents_owner_idx ON documents (owner_id, id);

-- A post attaches documents by ID, without a foreign key: a deleted
-- document leaves the post. Documents are content enough for a post.
ALTER TABLE posts
    ADD COLUMN document_ids uuid[] NOT NULL DEFAULT '{}' CHECK (cardinality(document_ids) <= 10),
    DROP CONSTRAINT posts_content_check,
    ADD CONSTRAINT posts_content_check
        CHECK (text <> '' OR cardinality(photo_ids) > 0 OR repost_of IS NOT NULL OR shared_photo_id IS NOT NULL OR polled
            OR cardinality(document_ids) > 0);

-- +goose Down
DELETE FROM posts
WHERE text = '' AND cardinality(photo_ids) = 0 AND repost_of IS NULL AND shared_photo_id IS NULL AND NOT polled;

ALTER TABLE posts
    DROP CONSTRAINT posts_content_check,
    ADD CONSTRAINT posts_content_check
        CHECK (text <> '' OR cardinality(photo_ids) > 0 OR repost_of IS NOT NULL OR shared_photo_id IS NOT NULL OR polled),
    DROP COLUMN document_ids;

DROP TABLE documents;
