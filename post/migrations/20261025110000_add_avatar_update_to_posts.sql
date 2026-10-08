-- +goose Up
-- The post of a new photo of the wall's owner, a user or a community
-- ("обновил фотографию на своей странице" of openvk): that photo alone,
-- never edited.
ALTER TABLE posts
    ADD COLUMN avatar_update boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT posts_avatar_update_check CHECK (
        NOT avatar_update OR (text = '' AND cardinality(photo_ids) = 1 AND repost_of IS NULL)
    );

-- +goose Down
ALTER TABLE posts
    DROP CONSTRAINT posts_avatar_update_check,
    DROP COLUMN avatar_update;
