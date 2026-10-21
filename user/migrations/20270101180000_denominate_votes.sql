-- +goose Up
-- The denomination of the votes, 1 : 1 000 000: every number of votes is
-- divided and rounded to the nearest, and whoever had more than nothing
-- keeps at least 1. The balances, the vouchers, the stakes on events, the
-- rounds and the stats of the casino are divided alike; the events taking
-- stakes keep their odds.
UPDATE users
SET votes = CASE WHEN votes > 0 THEN greatest(1, round(votes / 1000000.0))::bigint ELSE 0 END
WHERE votes <> 0;

-- A voucher with votes covers a number of usages now: one for any number
-- has run out.
UPDATE vouchers
SET
    votes = CASE WHEN votes > 0 THEN greatest(1, round(votes / 1000000.0))::bigint ELSE 0 END,
    usages_left = CASE WHEN votes > 0 AND usages_left IS NULL THEN 0 ELSE usages_left END;

UPDATE bet_stakes
SET
    amount = greatest(1, round(amount / 1000000.0))::bigint,
    potential = CASE WHEN potential > 0 THEN greatest(1, round(potential / 1000000.0))::bigint ELSE 0 END,
    payout = CASE WHEN payout > 0 THEN greatest(1, round(payout / 1000000.0))::bigint ELSE 0 END;

UPDATE rocket_rounds
SET
    amount = greatest(1, round(amount / 1000000.0))::bigint,
    payout = CASE WHEN payout > 0 THEN greatest(1, round(payout / 1000000.0))::bigint ELSE 0 END;

-- The casino takes the settings of the new scale: it gives back 85%, a
-- round wins at most 1000.
UPDATE casino
SET
    spent = round(spent / 1000000.0)::bigint,
    won = round(won / 1000000.0)::bigint,
    bet_spent = round(bet_spent / 1000000.0)::bigint,
    bet_won = round(bet_won / 1000000.0)::bigint,
    rocket_spent = round(rocket_spent / 1000000.0)::bigint,
    rocket_won = round(rocket_won / 1000000.0)::bigint,
    price = 10,
    prizes = '[
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
    ]',
    bet_least = 1,
    bet_most = 100,
    bet_aim = '[
        {"multiplier": 200, "chance": 4250},
        {"multiplier": 300, "chance": 2833},
        {"multiplier": 400, "chance": 2125},
        {"multiplier": 500, "chance": 1700},
        {"multiplier": 700, "chance": 1214},
        {"multiplier": 1000, "chance": 850}
    ]',
    rocket_least = 1,
    rocket_most = 100,
    rocket_return = 8500,
    rocket_top = 1000;

ALTER TABLE casino
    ALTER COLUMN rocket_most SET DEFAULT 100,
    ALTER COLUMN rocket_return SET DEFAULT 8500,
    ALTER COLUMN rocket_top SET DEFAULT 1000;

-- The statements start over: their lines are of the old scale. Each page
-- with votes opens with its balance.
DELETE FROM vote_operations;

ALTER TABLE vote_operations
    DROP CONSTRAINT vote_operations_kind_check,
    ADD CONSTRAINT vote_operations_kind_check
        CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake', 'rocket', 'bonus', 'denomination'));

INSERT INTO vote_operations (id, user_id, kind, amount, balance, note)
SELECT uuidv7(), id, 'denomination', votes, votes, '1 : 1 000 000'
FROM users
WHERE votes > 0;

-- +goose Down
DELETE FROM vote_operations;

ALTER TABLE vote_operations
    DROP CONSTRAINT vote_operations_kind_check,
    ADD CONSTRAINT vote_operations_kind_check
        CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake', 'rocket'));

ALTER TABLE casino
    ALTER COLUMN rocket_most SET DEFAULT 1000,
    ALTER COLUMN rocket_return SET DEFAULT 9500,
    ALTER COLUMN rocket_top SET DEFAULT 100000;

UPDATE casino
SET
    spent = spent * 1000000,
    won = won * 1000000,
    bet_spent = bet_spent * 1000000,
    bet_won = bet_won * 1000000,
    rocket_spent = rocket_spent * 1000000,
    rocket_won = rocket_won * 1000000;

UPDATE rocket_rounds
SET
    amount = amount * 1000000,
    payout = payout * 1000000;

UPDATE bet_stakes
SET
    amount = amount * 1000000,
    potential = potential * 1000000,
    payout = payout * 1000000;

UPDATE vouchers
SET votes = votes * 1000000;

UPDATE users
SET votes = votes * 1000000;
