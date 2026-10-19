-- +goose Up
-- What a vote is worth in rubles, in kopecks: one row, which the
-- administrators change.
CREATE TABLE vote_rates (
    only_row boolean PRIMARY KEY DEFAULT true CHECK (only_row),
    kopecks bigint NOT NULL CHECK (kopecks BETWEEN 1 AND 100000000)
);

INSERT INTO vote_rates (kopecks) VALUES (700);

-- +goose Down
DROP TABLE vote_rates;
