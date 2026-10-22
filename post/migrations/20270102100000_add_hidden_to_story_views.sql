-- +goose Up
-- A view by someone with Freenet Prime marks the story viewed for them but
-- is not told to its author.
ALTER TABLE story_views
    ADD COLUMN hidden boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE story_views
    DROP COLUMN hidden;
