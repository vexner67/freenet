-- +goose Up
-- openvk's "Фон страницы" of a person: photos of the media service kept
-- under the user, shown on the left and on the right of the page.
ALTER TABLE users
    ADD COLUMN backdrop_left uuid,
    ADD COLUMN backdrop_right uuid;

-- +goose Down
ALTER TABLE users
    DROP COLUMN backdrop_right,
    DROP COLUMN backdrop_left;
