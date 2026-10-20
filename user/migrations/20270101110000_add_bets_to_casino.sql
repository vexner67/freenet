-- +goose Up
-- The bets of the casino: a player bets from bet_least to bet_most votes
-- on a multiplier and wins it with the chance bet_return percent divided
-- by it; bets, bet_spent and bet_won are how they do.
ALTER TABLE casino
    ADD COLUMN bet_least bigint NOT NULL DEFAULT 1,
    ADD COLUMN bet_most bigint NOT NULL DEFAULT 1000,
    ADD COLUMN bet_return integer NOT NULL DEFAULT 95 CHECK (bet_return BETWEEN 1 AND 100),
    ADD COLUMN bets bigint NOT NULL DEFAULT 0 CHECK (bets >= 0),
    ADD COLUMN bet_spent bigint NOT NULL DEFAULT 0 CHECK (bet_spent >= 0),
    ADD COLUMN bet_won bigint NOT NULL DEFAULT 0 CHECK (bet_won >= 0),
    ADD CONSTRAINT casino_bet_limits CHECK (bet_least BETWEEN 1 AND bet_most AND bet_most <= 1000000);

-- +goose Down
ALTER TABLE casino
    DROP CONSTRAINT casino_bet_limits,
    DROP COLUMN bet_least,
    DROP COLUMN bet_most,
    DROP COLUMN bet_return,
    DROP COLUMN bets,
    DROP COLUMN bet_spent,
    DROP COLUMN bet_won;
