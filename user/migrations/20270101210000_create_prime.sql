-- +goose Up
-- Freenet Prime, the subscription of the site: until when a page has it
-- (NULL: never bought) and whether its owner hides when they are online.
ALTER TABLE users
    ADD COLUMN prime_until     timestamptz,
    ADD COLUMN prime_invisible boolean NOT NULL DEFAULT false,
    DROP CONSTRAINT users_theme_check,
    ADD CONSTRAINT users_theme_check CHECK (theme IN ('default', 'blue', 'dark', 'gold'));

-- What the periods of Prime cost, in votes: one row, which the
-- administrators change.
CREATE TABLE prime_prices (
    only_row boolean PRIMARY KEY DEFAULT true CHECK (only_row),
    month    bigint  NOT NULL CHECK (month BETWEEN 1 AND 1000000),
    quarter  bigint  NOT NULL CHECK (quarter BETWEEN 1 AND 1000000),
    year     bigint  NOT NULL CHECK (year BETWEEN 1 AND 1000000)
);

INSERT INTO prime_prices (month, quarter, year) VALUES (150, 400, 1500);

-- A purchase of Prime is a line of the statement; the note is its period.
ALTER TABLE vote_operations
    DROP CONSTRAINT vote_operations_kind_check,
    ADD CONSTRAINT vote_operations_kind_check
        CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake', 'rocket', 'bonus', 'denomination', 'fee', 'tax', 'rating', 'prime'));

-- +goose Down
DELETE FROM vote_operations
WHERE kind = 'prime';

ALTER TABLE vote_operations
    DROP CONSTRAINT vote_operations_kind_check,
    ADD CONSTRAINT vote_operations_kind_check
        CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake', 'rocket', 'bonus', 'denomination', 'fee', 'tax', 'rating'));

DROP TABLE prime_prices;

UPDATE users
SET theme = 'default'
WHERE theme = 'gold';

ALTER TABLE users
    DROP CONSTRAINT users_theme_check,
    ADD CONSTRAINT users_theme_check CHECK (theme IN ('default', 'blue', 'dark')),
    DROP COLUMN prime_invisible,
    DROP COLUMN prime_until;
