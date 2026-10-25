-- +goose Up
-- "Покер" of the casino, one row: the table (NULL — user.DefaultPoker)
-- and how its hands do — dealt, the votes bet (antes and calls) and paid
-- back.
CREATE TABLE poker (
    only_row boolean PRIMARY KEY DEFAULT true CHECK (only_row),
    settings jsonb CHECK (settings IS NULL OR jsonb_typeof(settings) = 'object'),
    hands bigint NOT NULL DEFAULT 0 CHECK (hands >= 0),
    spent bigint NOT NULL DEFAULT 0 CHECK (spent >= 0),
    won bigint NOT NULL DEFAULT 0 CHECK (won >= 0)
);

INSERT INTO poker DEFAULT VALUES;

-- The hands of "Покер" (user.PokerHand): the nine cards drawn at the deal
-- (the player's two, the dealer's two, the board's five; 0 to 51), where
-- the hand is and what it paid back. A player has one hand dealt at a
-- time; a deal deletes their hands over older than a day.
CREATE TABLE poker_hands (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    ante bigint NOT NULL CHECK (ante > 0),
    cards smallint[] NOT NULL CHECK (cardinality(cards) = 9 AND 0 <= ALL (cards) AND 51 >= ALL (cards)),
    state text NOT NULL CHECK (state IN ('dealt', 'folded', 'won', 'lost', 'tied', 'unqualified')),
    payout bigint NOT NULL DEFAULT 0 CHECK (payout >= 0),
    started_at timestamptz NOT NULL
);

CREATE UNIQUE INDEX poker_hands_dealt_idx ON poker_hands (user_id) WHERE state = 'dealt';
CREATE INDEX poker_hands_user_id_idx ON poker_hands (user_id, started_at);

ALTER TABLE vote_operations
    DROP CONSTRAINT vote_operations_kind_check,
    ADD CONSTRAINT vote_operations_kind_check
        CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake', 'rocket', 'bonus', 'denomination', 'fee', 'tax', 'rating', 'prime',
                        'gift_upgrade', 'gift_transfer', 'gift_sale', 'gift_fee', 'gift_bid', 'gift_refund', 'mine', 'mine_chest', 'post', 'message',
                        'business', 'deposit', 'loan', 'poker'));

-- +goose Down
DELETE FROM vote_operations
WHERE kind = 'poker';

ALTER TABLE vote_operations
    DROP CONSTRAINT vote_operations_kind_check,
    ADD CONSTRAINT vote_operations_kind_check
        CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake', 'rocket', 'bonus', 'denomination', 'fee', 'tax', 'rating', 'prime',
                        'gift_upgrade', 'gift_transfer', 'gift_sale', 'gift_fee', 'gift_bid', 'gift_refund', 'mine', 'mine_chest', 'post', 'message',
                        'business', 'deposit', 'loan'));

DROP TABLE poker_hands;
DROP TABLE poker;
