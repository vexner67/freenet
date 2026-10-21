-- +goose Up
-- "Ракетка" of the casino: a player bets from rocket_least to rocket_most
-- votes, and a rocket flies with a multiplier growing from ×1 until a crash
-- point drawn at the launch — at x or later with the chance rocket_return
-- (hundredths of a percent) / x, never past rocket_top (hundredths). The
-- bet cashed before the crash comes back multiplied. rockets, rocket_spent
-- and rocket_won are how the rounds do.
ALTER TABLE casino
    ADD COLUMN rocket_least bigint NOT NULL DEFAULT 1,
    ADD COLUMN rocket_most bigint NOT NULL DEFAULT 1000,
    ADD COLUMN rocket_return integer NOT NULL DEFAULT 9500 CHECK (rocket_return BETWEEN 1 AND 10000),
    ADD COLUMN rocket_top integer NOT NULL DEFAULT 100000 CHECK (rocket_top BETWEEN 200 AND 1000000),
    ADD COLUMN rockets bigint NOT NULL DEFAULT 0 CHECK (rockets >= 0),
    ADD COLUMN rocket_spent bigint NOT NULL DEFAULT 0 CHECK (rocket_spent >= 0),
    ADD COLUMN rocket_won bigint NOT NULL DEFAULT 0 CHECK (rocket_won >= 0),
    ADD CONSTRAINT casino_rocket_limits CHECK (rocket_least BETWEEN 1 AND rocket_most AND rocket_most <= 100000000);

-- The rounds of the rocket: crash, auto and cashed in hundredths. auto is
-- the multiplier the round cashes at by itself (NULL — by hand), cashed
-- the one it was cashed at (NULL — not cashed), payout what it paid. A
-- launch deletes the rounds of its player older than an hour.
CREATE TABLE rocket_rounds (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    amount bigint NOT NULL CHECK (amount > 0),
    crash integer NOT NULL CHECK (crash BETWEEN 100 AND 1000000),
    auto integer CHECK (auto BETWEEN 101 AND 1000000),
    started_at timestamptz NOT NULL,
    cashed integer CHECK (cashed BETWEEN 100 AND 1000000 AND cashed < crash),
    payout bigint NOT NULL DEFAULT 0 CHECK (payout >= 0)
);

CREATE INDEX rocket_rounds_user_id_idx ON rocket_rounds (user_id, started_at);

ALTER TABLE vote_operations
    DROP CONSTRAINT vote_operations_kind_check,
    ADD CONSTRAINT vote_operations_kind_check
        CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake', 'rocket'));

-- +goose Down
DELETE FROM vote_operations WHERE kind = 'rocket';

ALTER TABLE vote_operations
    DROP CONSTRAINT vote_operations_kind_check,
    ADD CONSTRAINT vote_operations_kind_check
        CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake'));

DROP TABLE rocket_rounds;

ALTER TABLE casino
    DROP CONSTRAINT casino_rocket_limits,
    DROP COLUMN rocket_least,
    DROP COLUMN rocket_most,
    DROP COLUMN rocket_return,
    DROP COLUMN rocket_top,
    DROP COLUMN rockets,
    DROP COLUMN rocket_spent,
    DROP COLUMN rocket_won;
