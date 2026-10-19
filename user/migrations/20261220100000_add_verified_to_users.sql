-- +goose Up
-- The checkmark of a page ("Верификация" of openvk), given and taken by
-- the administrators of the site.
ALTER TABLE users
    ADD COLUMN verified boolean NOT NULL DEFAULT false;

-- The administrators' list of the verified pages, newest first.
CREATE INDEX users_verified_idx ON users (id DESC) WHERE verified;

-- +goose Down
DROP INDEX users_verified_idx;

ALTER TABLE users
    DROP COLUMN verified;
