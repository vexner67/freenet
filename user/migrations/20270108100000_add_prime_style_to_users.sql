-- +goose Up
-- How a member of Freenet Prime dresses their page: the colour of the
-- name, the frame of the photo, the pattern behind the page and the mood
-- by the status. The site draws each; others see it while Prime lasts.
ALTER TABLE users
    ADD COLUMN prime_name text NOT NULL DEFAULT 'amethyst'
        CHECK (prime_name IN ('amethyst', 'ruby', 'emerald', 'sapphire', 'gold', 'rainbow')),
    ADD COLUMN prime_frame text NOT NULL DEFAULT 'amethyst'
        CHECK (prime_frame IN ('amethyst', 'gold', 'silver', 'glitter', 'hearts', 'stars')),
    ADD COLUMN prime_background text NOT NULL DEFAULT ''
        CHECK (prime_background IN ('', 'stars', 'hearts', 'checks', 'clouds', 'flowers', 'sparkles')),
    ADD COLUMN prime_mood text NOT NULL DEFAULT ''
        CHECK (prime_mood IN ('', 'happy', 'love', 'cool', 'wink', 'sad', 'angry', 'sleepy'));

-- +goose Down
ALTER TABLE users
    DROP COLUMN prime_mood,
    DROP COLUMN prime_background,
    DROP COLUMN prime_frame,
    DROP COLUMN prime_name;
