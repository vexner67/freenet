-- +goose Up
-- Prime is sold by the month and the year only. Statements keep the
-- quarters bought before: their note is still "quarter".
ALTER TABLE prime_prices
    DROP COLUMN quarter;

-- +goose Down
ALTER TABLE prime_prices
    ADD COLUMN quarter bigint NOT NULL DEFAULT 400 CHECK (quarter >= 1);

ALTER TABLE prime_prices
    ALTER COLUMN quarter DROP DEFAULT;
