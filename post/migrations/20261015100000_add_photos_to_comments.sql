-- +goose Up
-- As on posts: photo_ids refer to the media service, so there is no
-- foreign key; the gateway checks they are the author's.
ALTER TABLE comments
    ADD COLUMN photo_ids uuid[] NOT NULL DEFAULT '{}' CHECK (cardinality(photo_ids) <= 10);

-- A comment may now be photos only, but never empty.
ALTER TABLE comments
    DROP CONSTRAINT comments_text_check,
    ADD CONSTRAINT comments_content_check CHECK (text <> '' OR cardinality(photo_ids) > 0);

-- +goose Down
-- Comments of photos only cannot exist without the column; they leave
-- their posts' counters too.
WITH deleted AS (
    DELETE FROM comments
    WHERE text = ''
    RETURNING post_id
)
UPDATE posts p
SET comment_count = p.comment_count - d.n
FROM (
    SELECT post_id, count(*) AS n
    FROM deleted
    GROUP BY post_id
) d
WHERE p.id = d.post_id;

ALTER TABLE comments
    DROP CONSTRAINT comments_content_check,
    ADD CONSTRAINT comments_text_check CHECK (text <> '');

ALTER TABLE comments
    DROP COLUMN photo_ids;
