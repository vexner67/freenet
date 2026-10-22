-- +goose Up
-- Collectible gifts, as Telegram's: a number of its kind, three traits
-- (model, backdrop, symbol) and, while it is on sale, a price. A gift the
-- site holds to auction has no recipient.
ALTER TABLE gifts
    ALTER COLUMN recipient_id DROP NOT NULL,
    ADD COLUMN number     integer CHECK (number > 0),
    ADD COLUMN model      text,
    ADD COLUMN backdrop   text,
    ADD COLUMN symbol     text,
    ADD COLUMN sale_price bigint CHECK (sale_price BETWEEN 1 AND 1000000),
    ADD CONSTRAINT gifts_collectible_check CHECK (
        (number IS NULL) = (model IS NULL)
        AND (number IS NULL) = (backdrop IS NULL)
        AND (number IS NULL) = (symbol IS NULL)),
    ADD CONSTRAINT gifts_sale_check CHECK (sale_price IS NULL OR number IS NOT NULL),
    ADD CONSTRAINT gifts_recipient_check CHECK (recipient_id IS NOT NULL OR number IS NOT NULL);

-- A number is one collectible of its kind.
CREATE UNIQUE INDEX gifts_number_key ON gifts (gift, number) WHERE number IS NOT NULL;
-- The market: what is on sale, cheapest first.
CREATE INDEX gifts_sale_idx ON gifts (sale_price, id) WHERE sale_price IS NOT NULL;

-- How many of a gift there may be (0: no end), have been sent and have
-- been made collectible.
CREATE TABLE gift_stock (
    gift   text    PRIMARY KEY CHECK (gift <> ''),
    supply integer NOT NULL DEFAULT 0 CHECK (supply BETWEEN 0 AND 1000000),
    sold   integer NOT NULL DEFAULT 0 CHECK (sold >= 0),
    minted integer NOT NULL DEFAULT 0 CHECK (minted >= 0)
);

INSERT INTO gift_stock (gift, sold)
SELECT gift, count(*)
FROM gifts
GROUP BY gift;

-- An auction of a collectible: its seller (NULL: the site), the start, the
-- end and the top bid, whose votes are held from its bidder.
CREATE TABLE gift_auctions (
    gift_id     uuid        PRIMARY KEY REFERENCES gifts (id) ON DELETE CASCADE,
    seller_id   uuid        REFERENCES users (id) ON DELETE CASCADE,
    start_price bigint      NOT NULL CHECK (start_price BETWEEN 1 AND 1000000),
    ends_at     timestamptz NOT NULL,
    top_bid     bigint      NOT NULL DEFAULT 0 CHECK (top_bid BETWEEN 0 AND 1000000),
    bidder_id   uuid        REFERENCES users (id) ON DELETE SET NULL,
    bids        integer     NOT NULL DEFAULT 0 CHECK (bids >= 0)
);

-- The auctions running and those due, ending soonest first.
CREATE INDEX gift_auctions_ends_at_idx ON gift_auctions (ends_at, gift_id);
-- The auctions an erased page bid on.
CREATE INDEX gift_auctions_bidder_id_idx ON gift_auctions (bidder_id) WHERE bidder_id IS NOT NULL;

-- The history of a collectible: from whom to whom (NULL: the site, or an
-- erased page) and for how much.
CREATE TABLE gift_history (
    id      uuid   PRIMARY KEY,
    gift_id uuid   NOT NULL REFERENCES gifts (id) ON DELETE CASCADE,
    kind    text   NOT NULL CHECK (kind IN ('upgrade', 'mint', 'transfer', 'sale', 'auction')),
    from_id uuid   REFERENCES users (id) ON DELETE SET NULL,
    to_id   uuid   REFERENCES users (id) ON DELETE SET NULL,
    price   bigint NOT NULL DEFAULT 0 CHECK (price >= 0)
);

CREATE INDEX gift_history_gift_id_idx ON gift_history (gift_id, id DESC);
CREATE INDEX gift_history_from_id_idx ON gift_history (from_id) WHERE from_id IS NOT NULL;
CREATE INDEX gift_history_to_id_idx ON gift_history (to_id) WHERE to_id IS NOT NULL;

-- The upgrade, the transfer, the sale, its fee, a bid and its refund are
-- lines of the statement; the note names the gift and its number.
ALTER TABLE vote_operations
    DROP CONSTRAINT vote_operations_kind_check,
    ADD CONSTRAINT vote_operations_kind_check
        CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake', 'rocket', 'bonus', 'denomination', 'fee', 'tax', 'rating', 'prime',
                        'gift_upgrade', 'gift_transfer', 'gift_sale', 'gift_fee', 'gift_bid', 'gift_refund'));

-- +goose Down
DELETE FROM vote_operations
WHERE kind IN ('gift_upgrade', 'gift_transfer', 'gift_sale', 'gift_fee', 'gift_bid', 'gift_refund');

ALTER TABLE vote_operations
    DROP CONSTRAINT vote_operations_kind_check,
    ADD CONSTRAINT vote_operations_kind_check
        CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake', 'rocket', 'bonus', 'denomination', 'fee', 'tax', 'rating', 'prime'));

DROP TABLE gift_history;
DROP TABLE gift_auctions;
DROP TABLE gift_stock;

DELETE FROM gifts
WHERE recipient_id IS NULL;

ALTER TABLE gifts
    DROP CONSTRAINT gifts_recipient_check,
    DROP CONSTRAINT gifts_sale_check,
    DROP CONSTRAINT gifts_collectible_check,
    DROP COLUMN sale_price,
    DROP COLUMN symbol,
    DROP COLUMN backdrop,
    DROP COLUMN model,
    DROP COLUMN number,
    ALTER COLUMN recipient_id SET NOT NULL;
