-- +goose Up
-- The mine of the site: one row with its settings (NULL — the defaults of
-- the code, user.DefaultMine) and how it does.
CREATE TABLE mine (
    only_row boolean PRIMARY KEY DEFAULT true CHECK (only_row),
    settings jsonb CHECK (settings IS NULL OR jsonb_typeof(settings) = 'object'),
    digs bigint NOT NULL DEFAULT 0 CHECK (digs >= 0),
    sold bigint NOT NULL DEFAULT 0 CHECK (sold >= 0),
    chests bigint NOT NULL DEFAULT 0 CHECK (chests >= 0)
);

INSERT INTO mine DEFAULT VALUES;

-- A page in the mine: its energy as counted at counted_at, its experience
-- and the ore it has not sold, by ore.
CREATE TABLE miners (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    energy integer NOT NULL CHECK (energy >= 0),
    counted_at timestamptz NOT NULL,
    experience bigint NOT NULL DEFAULT 0 CHECK (experience >= 0),
    backpack jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(backpack) = 'object'),
    digs bigint NOT NULL DEFAULT 0 CHECK (digs >= 0),
    earned bigint NOT NULL DEFAULT 0 CHECK (earned >= 0)
);

-- Ore sold and chests found are lines of the statement; the note is the
-- ore.
ALTER TABLE vote_operations
    DROP CONSTRAINT vote_operations_kind_check,
    ADD CONSTRAINT vote_operations_kind_check
        CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake', 'rocket', 'bonus', 'denomination', 'fee', 'tax', 'rating', 'prime',
                        'gift_upgrade', 'gift_transfer', 'gift_sale', 'gift_fee', 'gift_bid', 'gift_refund', 'mine', 'mine_chest'));

-- +goose Down
DELETE FROM vote_operations
WHERE kind IN ('mine', 'mine_chest');

ALTER TABLE vote_operations
    DROP CONSTRAINT vote_operations_kind_check,
    ADD CONSTRAINT vote_operations_kind_check
        CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake', 'rocket', 'bonus', 'denomination', 'fee', 'tax', 'rating', 'prime',
                        'gift_upgrade', 'gift_transfer', 'gift_sale', 'gift_fee', 'gift_bid', 'gift_refund'));

DROP TABLE miners;
DROP TABLE mine;
