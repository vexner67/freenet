-- +goose Up
-- "под кайфом": a Prime pattern of melting rainbows and mushrooms.
ALTER TABLE users
    DROP CONSTRAINT users_prime_background_check,
    ADD CONSTRAINT users_prime_background_check
        CHECK (prime_background IN ('', 'stars', 'hearts', 'checks', 'clouds', 'flowers', 'sparkles', 'trip'));

-- +goose Down
UPDATE users SET prime_background = '' WHERE prime_background = 'trip';
ALTER TABLE users
    DROP CONSTRAINT users_prime_background_check,
    ADD CONSTRAINT users_prime_background_check
        CHECK (prime_background IN ('', 'stars', 'hearts', 'checks', 'clouds', 'flowers', 'sparkles'));
