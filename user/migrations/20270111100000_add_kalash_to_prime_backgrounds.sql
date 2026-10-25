-- +goose Up
-- Kalashnikovs join the patterns a member of Freenet Prime puts behind
-- the page.
ALTER TABLE users
    DROP CONSTRAINT users_prime_background_check,
    ADD CONSTRAINT users_prime_background_check
        CHECK (prime_background IN ('', 'stars', 'hearts', 'checks', 'clouds', 'flowers', 'sparkles', 'kalash'));

-- +goose Down
UPDATE users SET prime_background = '' WHERE prime_background = 'kalash';
ALTER TABLE users
    DROP CONSTRAINT users_prime_background_check,
    ADD CONSTRAINT users_prime_background_check
        CHECK (prime_background IN ('', 'stars', 'hearts', 'checks', 'clouds', 'flowers', 'sparkles'));
