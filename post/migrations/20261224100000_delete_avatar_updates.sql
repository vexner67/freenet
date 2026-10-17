-- +goose Up
-- The site no longer posts a new photo of a page or a club on its wall
-- ("обновил(а) фотографию на своей странице"; the owner of the site
-- decided so), and the posts of the photos posted before go, as deleting
-- each would take it: likes, comments, pins and votes by cascade, the
-- photos in media and the notifications through the outbox.
--
-- The payloads are the proto requests of the RPCs: DeletePublishedPhotosRequest
-- {owner_id = 1, photo_ids = 2} and ForgetPostRequest {post_id = 1}, each
-- a string of 36 bytes (0x0a24 and 0x1224 are the tags and the length). A
-- post and a comment have at most 10 photos, as the request takes.
INSERT INTO outbox (stream, kind, payload)
SELECT 'media', 'delete_photos', p.payload
FROM (
    SELECT c.id,
        '\x0a24'::bytea || convert_to(c.author_id::text, 'UTF8') || (
            SELECT string_agg('\x1224'::bytea || convert_to(photo::text, 'UTF8'), ''::bytea ORDER BY n)
            FROM unnest(c.photo_ids) WITH ORDINALITY AS u(photo, n)
        ) AS payload
    FROM comments c
    JOIN posts ON posts.id = c.post_id
    WHERE posts.avatar_update
        AND cardinality(c.photo_ids) > 0
    UNION ALL
    SELECT posts.id,
        '\x0a24'::bytea || convert_to(posts.author_id::text, 'UTF8') || (
            SELECT string_agg('\x1224'::bytea || convert_to(photo::text, 'UTF8'), ''::bytea ORDER BY n)
            FROM unnest(posts.photo_ids) WITH ORDINALITY AS u(photo, n)
        ) AS payload
    FROM posts
    WHERE posts.avatar_update
        AND cardinality(posts.photo_ids) > 0
) AS p
ORDER BY p.id;

INSERT INTO outbox (stream, kind, payload)
SELECT 'notification', 'forget_post', '\x0a24'::bytea || convert_to(id::text, 'UTF8')
FROM posts
WHERE avatar_update
ORDER BY id;

DELETE FROM posts WHERE avatar_update;

ALTER TABLE posts
    DROP CONSTRAINT posts_avatar_update_check,
    DROP COLUMN avatar_update;

-- +goose Down
-- The posts are gone with their photos; nothing comes back.
ALTER TABLE posts
    ADD COLUMN avatar_update boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT posts_avatar_update_check CHECK (
        NOT avatar_update OR (text = '' AND cardinality(photo_ids) = 1 AND repost_of IS NULL)
    );
