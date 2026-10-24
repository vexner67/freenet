-- +goose Up
-- The businesses of the site: one row with their rules (NULL — the
-- defaults of the code, user.DefaultBusinessRules) and how they do.
CREATE TABLE business (
    only_row boolean PRIMARY KEY DEFAULT true CHECK (only_row),
    settings jsonb CHECK (settings IS NULL OR jsonb_typeof(settings) = 'object'),
    spent bigint NOT NULL DEFAULT 0 CHECK (spent >= 0),
    paid bigint NOT NULL DEFAULT 0 CHECK (paid >= 0)
);

INSERT INTO business DEFAULT VALUES;

-- A business of a page as counted at counted_at: cash is in hundredths of
-- a vote and below zero is a debt; condition is in hundredths of a percent.
CREATE TABLE businesses (
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('kiosk', 'cafe', 'carwash', 'factory')),
    level integer NOT NULL CHECK (level >= 1),
    stock bigint NOT NULL CHECK (stock >= 0),
    price bigint NOT NULL CHECK (price >= 1),
    staff integer NOT NULL CHECK (staff >= 0),
    condition integer NOT NULL CHECK (condition BETWEEN 0 AND 10000),
    cash bigint NOT NULL,
    ad_until timestamptz NOT NULL,
    counted_at timestamptz NOT NULL,
    bankrupt boolean NOT NULL DEFAULT false,
    invested bigint NOT NULL CHECK (invested >= 0),
    collected bigint NOT NULL DEFAULT 0 CHECK (collected >= 0),
    PRIMARY KEY (user_id, kind)
);

-- Votes spent on a business and taken out of it are lines of the
-- statement; the note is the business and the action.
ALTER TABLE vote_operations
    DROP CONSTRAINT vote_operations_kind_check,
    ADD CONSTRAINT vote_operations_kind_check
        CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake', 'rocket', 'bonus', 'denomination', 'fee', 'tax', 'rating', 'prime',
                        'gift_upgrade', 'gift_transfer', 'gift_sale', 'gift_fee', 'gift_bid', 'gift_refund', 'mine', 'mine_chest', 'post', 'message',
                        'business'));

-- +goose Down
DELETE FROM vote_operations
WHERE kind = 'business';

ALTER TABLE vote_operations
    DROP CONSTRAINT vote_operations_kind_check,
    ADD CONSTRAINT vote_operations_kind_check
        CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake', 'rocket', 'bonus', 'denomination', 'fee', 'tax', 'rating', 'prime',
                        'gift_upgrade', 'gift_transfer', 'gift_sale', 'gift_fee', 'gift_bid', 'gift_refund', 'mine', 'mine_chest', 'post', 'message'));

DROP TABLE businesses;
DROP TABLE business;
