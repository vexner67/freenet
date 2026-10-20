-- +goose Up
-- The orange plaque of openvk ("user-alert") the administrators of the
-- site put on a page: "иностранный агент".
ALTER TABLE users
    ADD COLUMN foreign_agent boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE users
    DROP COLUMN foreign_agent;
