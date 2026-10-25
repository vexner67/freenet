-- +goose Up
-- The Kalashnikovs behind the page give way to an Islamic pattern: those
-- who chose them get it.
ALTER TABLE users DROP CONSTRAINT users_prime_background_check;
UPDATE users SET prime_background = 'islam' WHERE prime_background = 'kalash';
ALTER TABLE users
    ADD CONSTRAINT users_prime_background_check
        CHECK (prime_background IN ('', 'stars', 'hearts', 'checks', 'clouds', 'flowers', 'sparkles', 'islam'));

-- +goose Down
ALTER TABLE users DROP CONSTRAINT users_prime_background_check;
UPDATE users SET prime_background = 'kalash' WHERE prime_background = 'islam';
ALTER TABLE users
    ADD CONSTRAINT users_prime_background_check
        CHECK (prime_background IN ('', 'stars', 'hearts', 'checks', 'clouds', 'flowers', 'sparkles', 'kalash'));
