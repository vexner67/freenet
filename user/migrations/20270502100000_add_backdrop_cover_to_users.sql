-- +goose Up
-- A photo of one's own behind the whole page, a thing of Freenet Prime;
-- kept when Prime ends, shown again when it comes back.
ALTER TABLE users
    ADD COLUMN backdrop_cover uuid;

-- +goose Down
ALTER TABLE users
    DROP COLUMN backdrop_cover;
