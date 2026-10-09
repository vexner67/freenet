-- +goose Up
-- openvk's likes, comments and "Поделиться" of a photo of an album. The
-- photo counts its likes and comments in the same statements that add and
-- remove them, as a post does.
ALTER TABLE album_photos
    ADD COLUMN like_count    integer NOT NULL DEFAULT 0 CHECK (like_count >= 0),
    ADD COLUMN comment_count integer NOT NULL DEFAULT 0 CHECK (comment_count >= 0);

CREATE TABLE album_photo_likes (
    photo_id   uuid        NOT NULL REFERENCES album_photos (id) ON DELETE CASCADE,
    user_id    uuid        NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (photo_id, user_id)
);

CREATE INDEX album_photo_likes_likers_idx ON album_photo_likes (photo_id, created_at DESC, user_id DESC);
-- Erasing a user finds their likes by it.
CREATE INDEX album_photo_likes_user_id_idx ON album_photo_likes (user_id);

-- Comments of a photo: text only, oldest first.
CREATE TABLE photo_comments (
    id         uuid        PRIMARY KEY,
    photo_id   uuid        NOT NULL REFERENCES album_photos (id) ON DELETE CASCADE,
    author_id  uuid        NOT NULL,
    text       text        NOT NULL CHECK (text <> ''),
    created_at timestamptz NOT NULL
);

CREATE INDEX photo_comments_photo_idx ON photo_comments (photo_id, id);
CREATE INDEX photo_comments_author_idx ON photo_comments (author_id);

-- A post sharing a photo of an album ("Поделиться" of a photo): its ID;
-- no foreign key, as with repost_of: a deleted photo leaves the post.
ALTER TABLE posts
    ADD COLUMN shared_photo_id uuid,
    DROP CONSTRAINT posts_content_check,
    ADD CONSTRAINT posts_content_check
        CHECK (text <> '' OR cardinality(photo_ids) > 0 OR repost_of IS NOT NULL OR shared_photo_id IS NOT NULL);

-- +goose Down
DELETE FROM posts WHERE text = '' AND cardinality(photo_ids) = 0 AND repost_of IS NULL;
ALTER TABLE posts
    DROP CONSTRAINT posts_content_check,
    ADD CONSTRAINT posts_content_check CHECK (text <> '' OR cardinality(photo_ids) > 0 OR repost_of IS NOT NULL),
    DROP COLUMN shared_photo_id;
DROP TABLE photo_comments;
DROP TABLE album_photo_likes;
ALTER TABLE album_photos
    DROP COLUMN comment_count,
    DROP COLUMN like_count;
