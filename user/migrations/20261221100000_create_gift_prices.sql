-- +goose Up
-- The prices the administrators of the site give gifts; a gift without
-- a row costs what its collection does.
CREATE TABLE gift_prices (
    gift text PRIMARY KEY CHECK (gift ~ '^[a-z0-9_]{1,32}$'),
    price bigint NOT NULL CHECK (price BETWEEN 0 AND 1000000)
);

-- +goose Down
DROP TABLE gift_prices;
