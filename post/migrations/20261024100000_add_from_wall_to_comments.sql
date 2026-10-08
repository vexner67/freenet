-- +goose Up
-- A comment that speaks for the community whose wall its post is on
-- ("От имени сообщества" of openvk).
ALTER TABLE comments ADD COLUMN from_wall boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE comments DROP COLUMN from_wall;
