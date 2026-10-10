-- +goose Up
-- A photo "Фотографии со стены" keeps of a post marked as adult is
-- blurred like the post.
ALTER TABLE album_photos
    ADD COLUMN nsfw boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE album_photos
    DROP COLUMN nsfw;
