-- +goose Up
-- Sensitive profile fields are hidden from other users until opened.
ALTER TABLE users
    ADD COLUMN show_birthday        boolean NOT NULL DEFAULT false,
    ADD COLUMN show_relationship    boolean NOT NULL DEFAULT false,
    ADD COLUMN show_worldview       boolean NOT NULL DEFAULT false,
    ADD COLUMN show_political_views boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE users
    DROP COLUMN show_birthday,
    DROP COLUMN show_relationship,
    DROP COLUMN show_worldview,
    DROP COLUMN show_political_views;
