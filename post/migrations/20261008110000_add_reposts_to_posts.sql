-- +goose Up
-- A repost shares repost_of on its author's wall. There is no foreign key:
-- a repost outlives the post it shared and shows "Запись удалена" then.
-- repost_count changes in the same statement as the repost.
ALTER TABLE posts
    ADD COLUMN repost_of    uuid,
    ADD COLUMN repost_count integer NOT NULL DEFAULT 0 CHECK (repost_count >= 0);

-- A repost may say nothing of its own.
ALTER TABLE posts
    DROP CONSTRAINT posts_content_check,
    ADD CONSTRAINT posts_content_check CHECK (text <> '' OR cardinality(photo_ids) > 0 OR repost_of IS NOT NULL);

-- +goose Down
-- Reposts without a word cannot exist without the column.
DELETE FROM posts WHERE text = '' AND cardinality(photo_ids) = 0;

ALTER TABLE posts
    DROP CONSTRAINT posts_content_check,
    ADD CONSTRAINT posts_content_check CHECK (text <> '' OR cardinality(photo_ids) > 0);

ALTER TABLE posts
    DROP COLUMN repost_count,
    DROP COLUMN repost_of;
