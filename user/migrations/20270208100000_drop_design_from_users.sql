-- +goose Up
-- The site has one look again: the modern design is gone.
ALTER TABLE users
    DROP COLUMN design;

-- +goose Down
ALTER TABLE users
    ADD COLUMN design text NOT NULL DEFAULT 'classic' CHECK (design IN ('classic', 'modern'));
