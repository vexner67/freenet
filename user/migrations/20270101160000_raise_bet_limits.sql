-- +goose Up
-- The bets of "Прицел" may be up to 100 000 000 votes, as a stake on an event.
ALTER TABLE casino
    DROP CONSTRAINT casino_bet_limits,
    ADD CONSTRAINT casino_bet_limits CHECK (bet_least BETWEEN 1 AND bet_most AND bet_most <= 100000000);

-- +goose Down
UPDATE casino
SET bet_most = LEAST(bet_most, 1000000),
    bet_least = LEAST(bet_least, 1000000);

ALTER TABLE casino
    DROP CONSTRAINT casino_bet_limits,
    ADD CONSTRAINT casino_bet_limits CHECK (bet_least BETWEEN 1 AND bet_most AND bet_most <= 1000000);
