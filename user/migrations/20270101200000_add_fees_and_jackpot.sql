-- +goose Up
-- The sinks of the votes: the fee of a transfer, the tax on a sale of a
-- sticker pack and rating bought go into the treasury, each a line of the
-- statement.
ALTER TABLE vote_operations
    DROP CONSTRAINT vote_operations_kind_check,
    ADD CONSTRAINT vote_operations_kind_check
        CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake', 'rocket', 'bonus', 'denomination', 'fee', 'tax', 'rating'));

-- The least win the casino shows as a jackpot on the whole screen.
ALTER TABLE casino
    ADD COLUMN jackpot bigint NOT NULL DEFAULT 1000 CHECK (jackpot BETWEEN 1 AND 1000);

-- +goose Down
ALTER TABLE casino
    DROP COLUMN jackpot;

DELETE FROM vote_operations
WHERE kind IN ('fee', 'tax', 'rating');

ALTER TABLE vote_operations
    DROP CONSTRAINT vote_operations_kind_check,
    ADD CONSTRAINT vote_operations_kind_check
        CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake', 'rocket', 'bonus', 'denomination'));
