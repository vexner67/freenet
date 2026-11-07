-- +goose Up
-- The rating of a page is how full it is (up to 100 percent) plus the
-- rating bought, at most a billion percent. score keeps it for the order
-- of the search, highest first: the weights are those of
-- completenessBlocks in internal/user/completeness.go, and they have to
-- stay the same.
UPDATE users SET rating = 1000000000 WHERE rating > 1000000000;

ALTER TABLE users
    ADD CONSTRAINT users_rating_max CHECK (rating <= 1000000000),
    ADD COLUMN score integer NOT NULL GENERATED ALWAYS AS (LEAST(
          (CASE WHEN avatar_id IS NULL THEN 0 ELSE 20 END)
        + (CASE WHEN status = '' THEN 0 ELSE 5 END)
        + (CASE WHEN surname = '' THEN 0 ELSE 3 END)
        + (CASE WHEN gender = '' THEN 0 ELSE 3 END)
        + (CASE WHEN birthday IS NULL THEN 0 ELSE 4 END)
        + (CASE WHEN hometown = '' THEN 0 ELSE 4 END)
        + (CASE WHEN relationship = '' THEN 0 ELSE 3 END)
        + (CASE WHEN worldview = '' THEN 0 ELSE 4 END)
        + (CASE WHEN political_views = '' THEN 0 ELSE 4 END)
        + (CASE WHEN contact_email = '' THEN 0 ELSE 4 END)
        + (CASE WHEN telegram = '' THEN 0 ELSE 4 END)
        + (CASE WHEN website = '' THEN 0 ELSE 4 END)
        + (CASE WHEN city = '' THEN 0 ELSE 4 END)
        + (CASE WHEN address = '' THEN 0 ELSE 4 END)
        + (CASE WHEN interests = '' THEN 0 ELSE 4 END)
        + (CASE WHEN fav_music = '' THEN 0 ELSE 4 END)
        + (CASE WHEN fav_films = '' THEN 0 ELSE 4 END)
        + (CASE WHEN fav_shows = '' THEN 0 ELSE 4 END)
        + (CASE WHEN fav_books = '' THEN 0 ELSE 4 END)
        + (CASE WHEN fav_quotes = '' THEN 0 ELSE 3 END)
        + (CASE WHEN fav_games = '' THEN 0 ELSE 3 END)
        + (CASE WHEN about = '' THEN 0 ELSE 4 END)
        + rating,
        1000000000)) STORED;

-- The search: the listed pages by their rating, then newest first.
CREATE INDEX users_score_idx ON users (score, id)
    WHERE nickname <> '' AND deactivated_at IS NULL;

-- +goose Down
DROP INDEX users_score_idx;

ALTER TABLE users
    DROP COLUMN score,
    DROP CONSTRAINT users_rating_max;
