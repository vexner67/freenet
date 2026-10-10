-- +goose Up
-- "Содержит NSFW-контент": the author marks a post as adult, and readers
-- see it blurred, or not in their news, as their settings say.
ALTER TABLE posts ADD COLUMN nsfw boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE posts DROP COLUMN nsfw;
