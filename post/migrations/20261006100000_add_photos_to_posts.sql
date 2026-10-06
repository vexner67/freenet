-- +goose Up
-- photo_ids refer to the media service, so there is no foreign key: the
-- gateway checks they are the author's before the post is written.
ALTER TABLE posts
    ADD COLUMN photo_ids uuid[] NOT NULL DEFAULT '{}' CHECK (cardinality(photo_ids) <= 10);

-- A post may now be photos only, but never empty.
ALTER TABLE posts
    DROP CONSTRAINT posts_text_check,
    ADD CONSTRAINT posts_content_check CHECK (text <> '' OR cardinality(photo_ids) > 0);

-- +goose Down
-- Posts of photos only cannot exist without the column.
DELETE FROM posts WHERE text = '';

ALTER TABLE posts
    DROP CONSTRAINT posts_content_check,
    ADD CONSTRAINT posts_text_check CHECK (text <> '');

ALTER TABLE posts
    DROP COLUMN photo_ids;
