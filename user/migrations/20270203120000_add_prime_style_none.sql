-- +goose Up
-- "Без цвета" and "без рамки": a member of Prime may leave the name and
-- the photo plain.
ALTER TABLE users
    DROP CONSTRAINT users_prime_name_check,
    DROP CONSTRAINT users_prime_frame_check;
ALTER TABLE users
    ADD CONSTRAINT users_prime_name_check
        CHECK (prime_name IN ('amethyst', 'ruby', 'emerald', 'sapphire', 'gold', 'rainbow', 'none')),
    ADD CONSTRAINT users_prime_frame_check
        CHECK (prime_frame IN ('amethyst', 'gold', 'silver', 'glitter', 'hearts', 'stars', 'none'));

-- +goose Down
UPDATE users SET prime_name = 'amethyst' WHERE prime_name = 'none';
UPDATE users SET prime_frame = 'amethyst' WHERE prime_frame = 'none';
ALTER TABLE users
    DROP CONSTRAINT users_prime_name_check,
    DROP CONSTRAINT users_prime_frame_check;
ALTER TABLE users
    ADD CONSTRAINT users_prime_name_check
        CHECK (prime_name IN ('amethyst', 'ruby', 'emerald', 'sapphire', 'gold', 'rainbow')),
    ADD CONSTRAINT users_prime_frame_check
        CHECK (prime_frame IN ('amethyst', 'gold', 'silver', 'glitter', 'hearts', 'stars'));
