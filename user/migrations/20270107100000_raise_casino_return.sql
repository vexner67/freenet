-- +goose Up
-- The casino gives back 97% of what is bet, as real casinos do: a game set
-- to give back less is raised to that, and the administrators may lower it
-- again. "Ракетка" — its return; "Прицел" — the chance of each multiplier,
-- at least 97% divided by it; "Огонёк" — the empty prizes weigh less, so
-- that the prizes give back 97% of the price (when they cannot, being
-- worth less than that even without the empty ones, they stay).
UPDATE casino
SET rocket_return = 9700
WHERE rocket_return < 9700;

ALTER TABLE casino
    ALTER COLUMN rocket_return SET DEFAULT 9700;

UPDATE casino
SET bet_aim = (
    SELECT jsonb_agg(
        jsonb_build_object(
            'multiplier', (m ->> 'multiplier')::int,
            'chance', LEAST(9999, GREATEST((m ->> 'chance')::int, 970000 / (m ->> 'multiplier')::int))
        )
        ORDER BY (m ->> 'multiplier')::int
    )
    FROM jsonb_array_elements(bet_aim) AS m
)
WHERE bet_aim IS NOT NULL;

-- The default prizes become the new default ones: the empty prizes weigh
-- 59500 each instead of 89500, and 10 votes twice as much.
UPDATE casino
SET prizes = '[
    {"votes": 5, "weight": 200000},
    {"votes": 0, "weight": 59500},
    {"votes": 1000, "weight": 1000},
    {"votes": 0, "weight": 59500},
    {"votes": 20, "weight": 80000},
    {"votes": 2, "weight": 200000},
    {"votes": 0, "weight": 59500},
    {"votes": 300, "weight": 3000},
    {"votes": 10, "weight": 240000},
    {"votes": 0, "weight": 59500},
    {"votes": 100, "weight": 10000},
    {"votes": 50, "weight": 28000}
]'
WHERE price = 10 AND prizes = '[
    {"votes": 5, "weight": 200000},
    {"votes": 0, "weight": 89500},
    {"votes": 1000, "weight": 1000},
    {"votes": 0, "weight": 89500},
    {"votes": 20, "weight": 80000},
    {"votes": 2, "weight": 200000},
    {"votes": 0, "weight": 89500},
    {"votes": 300, "weight": 3000},
    {"votes": 10, "weight": 120000},
    {"votes": 0, "weight": 89500},
    {"votes": 100, "weight": 10000},
    {"votes": 50, "weight": 28000}
]'::jsonb;

-- Prizes the administrators set: the empty ones weigh less.
WITH odds AS (
    SELECT c.price,
           sum((p ->> 'votes')::numeric * (p ->> 'weight')::numeric) AS won,
           sum((p ->> 'weight')::numeric) FILTER (WHERE (p ->> 'votes')::bigint > 0) AS paying,
           sum((p ->> 'weight')::numeric) FILTER (WHERE (p ->> 'votes')::bigint = 0) AS empty
    FROM casino AS c, jsonb_array_elements(c.prizes) AS p
    GROUP BY c.price
),
share AS (
    SELECT (won / (0.97 * price) - paying) / empty AS keep
    FROM odds
    WHERE empty > 0 AND paying > 0
)
UPDATE casino
SET prizes = (
    SELECT jsonb_agg(
        CASE
            WHEN (p ->> 'votes')::bigint = 0
                THEN jsonb_build_object('votes', 0, 'weight', floor((p ->> 'weight')::numeric * share.keep)::bigint)
            ELSE p
        END
        ORDER BY i
    )
    FROM jsonb_array_elements(casino.prizes) WITH ORDINALITY AS e (p, i)
)
FROM share
WHERE share.keep >= 0 AND share.keep < 1;

-- +goose Down
-- The settings stay: the administrators lower the return if they want to.
ALTER TABLE casino
    ALTER COLUMN rocket_return SET DEFAULT 8500;
