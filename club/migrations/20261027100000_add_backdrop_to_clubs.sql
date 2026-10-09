-- +goose Up
-- "Фон страницы" of openvk: photos of the club in the media service, shown
-- on the left and on the right of its page; NULL for none.
ALTER TABLE clubs
    ADD COLUMN backdrop_left uuid,
    ADD COLUMN backdrop_right uuid;

-- +goose Down
ALTER TABLE clubs
    DROP COLUMN backdrop_right,
    DROP COLUMN backdrop_left;
