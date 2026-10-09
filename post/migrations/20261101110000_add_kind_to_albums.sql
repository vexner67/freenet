-- +goose Up
-- Albums the site keeps: "avatars" is openvk's "Фотографии со страницы",
-- one an owner.
ALTER TABLE albums
    ADD COLUMN kind text NOT NULL DEFAULT ''
        CHECK (kind IN ('', 'avatars'));

CREATE UNIQUE INDEX albums_avatars_uidx ON albums (owner_id) WHERE kind = 'avatars';

-- +goose Down
DELETE FROM albums WHERE kind <> '';
DROP INDEX albums_avatars_uidx;
ALTER TABLE albums DROP COLUMN kind;
