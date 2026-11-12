-- +goose Up
-- The rating bought stops 100 percent short of the billion: the rest
-- comes only from filling the page in. Pages that bought past it lose
-- the excess (their fullness is counted on top, as before).
UPDATE users SET rating = 999999900 WHERE rating > 999999900;

ALTER TABLE users
    DROP CONSTRAINT users_rating_max,
    ADD CONSTRAINT users_rating_max CHECK (rating <= 999999900);

-- +goose Down
-- The percent taken away are not given back.
ALTER TABLE users
    DROP CONSTRAINT users_rating_max,
    ADD CONSTRAINT users_rating_max CHECK (rating <= 1000000000);
