-- +goose Up
-- The odds of the bets on events: each outcome has its odds, in hundredths
-- (250 — ×2.5), and a stake on the winner pays its votes times the odds it
-- was made at — potential, summed over the stakes of the row. Events made
-- before have no odds (NULL): those who guessed share the stakes.
ALTER TABLE bet_events
    ADD COLUMN odds integer[] CHECK (
        odds IS NULL
        OR (cardinality(odds) = cardinality(outcomes) AND 101 <= ALL (odds) AND 100000 >= ALL (odds))
    );

ALTER TABLE bet_stakes
    ADD COLUMN potential bigint NOT NULL DEFAULT 0 CHECK (potential >= 0);

-- The multipliers of "Прицел" and the chance of each, of 10000, set by the
-- administrators: [{"multiplier": 200, "chance": 4750}, ...]. They start as
-- the casino had them: each with the chance bet_return divided by it.
-- bet_return stays for the code of the deploy before; nothing reads it.
ALTER TABLE casino
    ADD COLUMN bet_aim jsonb CHECK (
        bet_aim IS NULL
        OR (jsonb_typeof(bet_aim) = 'array' AND jsonb_array_length(bet_aim) BETWEEN 1 AND 10)
    );

UPDATE casino
SET bet_aim = (
    SELECT jsonb_agg(jsonb_build_object('multiplier', m, 'chance', bet_return * 10000 / m) ORDER BY m)
    FROM unnest(ARRAY[150, 200, 300, 500, 1000]) AS m
);

-- +goose Down
ALTER TABLE casino
    DROP COLUMN bet_aim;

ALTER TABLE bet_stakes
    DROP COLUMN potential;

ALTER TABLE bet_events
    DROP COLUMN odds;
