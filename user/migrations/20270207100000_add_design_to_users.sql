-- +goose Up
-- "Оформление" of "Внешний вид": the site as it was in 2010, the default,
-- or the modern one.
ALTER TABLE users
    ADD COLUMN design text NOT NULL DEFAULT 'classic' CHECK (design IN ('classic', 'modern'));

-- +goose Down
ALTER TABLE users
    DROP COLUMN design;
