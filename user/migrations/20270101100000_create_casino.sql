-- +goose Up
-- The casino of the site: one row, which the administrators change. A spin
-- costs price votes and wins one of prizes ({"votes", "weight"}), each as
-- likely as its weight among all weights; spins, spent and won are how it
-- does.
CREATE TABLE casino (
    only_row boolean PRIMARY KEY DEFAULT true CHECK (only_row),
    price bigint NOT NULL CHECK (price BETWEEN 1 AND 10000),
    prizes jsonb NOT NULL CHECK (
        jsonb_typeof(prizes) = 'array'
        AND jsonb_array_length(prizes) BETWEEN 2 AND 12
    ),
    spins bigint NOT NULL DEFAULT 0 CHECK (spins >= 0),
    spent bigint NOT NULL DEFAULT 0 CHECK (spent >= 0),
    won bigint NOT NULL DEFAULT 0 CHECK (won >= 0)
);

INSERT INTO casino (price, prizes) VALUES (10, '[
    {"votes": 0, "weight": 50},
    {"votes": 5, "weight": 25},
    {"votes": 10, "weight": 12},
    {"votes": 20, "weight": 8},
    {"votes": 50, "weight": 4},
    {"votes": 100, "weight": 1}
]');

-- +goose Down
DROP TABLE casino;
