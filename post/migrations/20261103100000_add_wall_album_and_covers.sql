-- +goose Up
-- "wall" is openvk's "Фотографии со стены", one an owner as "avatars".
-- cover_photo_id is the photo chosen to show an album ("Обложка альбома"),
-- NULL while its first photo does; a photo deleted or moved away stops
-- showing it.
ALTER TABLE albums
    DROP CONSTRAINT albums_kind_check,
    ADD CONSTRAINT albums_kind_check CHECK (kind IN ('', 'avatars', 'wall')),
    ADD COLUMN cover_photo_id uuid REFERENCES album_photos (id) ON DELETE SET NULL;

CREATE UNIQUE INDEX albums_wall_uidx ON albums (owner_id) WHERE kind = 'wall';

-- +goose Down
DELETE FROM albums WHERE kind = 'wall';
DROP INDEX albums_wall_uidx;
ALTER TABLE albums
    DROP COLUMN cover_photo_id,
    DROP CONSTRAINT albums_kind_check,
    ADD CONSTRAINT albums_kind_check CHECK (kind IN ('', 'avatars'));
