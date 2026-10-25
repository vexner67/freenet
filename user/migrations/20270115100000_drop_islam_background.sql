-- +goose Up
-- The Islamic pattern behind the page is gone: those who chose it get the
-- plain page.
ALTER TABLE users DROP CONSTRAINT users_prime_background_check;
UPDATE users SET prime_background = '' WHERE prime_background = 'islam';
ALTER TABLE users
    ADD CONSTRAINT users_prime_background_check
        CHECK (prime_background IN ('', 'stars', 'hearts', 'checks', 'clouds', 'flowers', 'sparkles'));

-- +goose Down
ALTER TABLE users DROP CONSTRAINT users_prime_background_check;
ALTER TABLE users
    ADD CONSTRAINT users_prime_background_check
        CHECK (prime_background IN ('', 'stars', 'hearts', 'checks', 'clouds', 'flowers', 'sparkles', 'islam'));
