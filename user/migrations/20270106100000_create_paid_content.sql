-- +goose Up
-- Who writes to the user: only people with Freenet Prime, and for how many
-- votes a message (0 — free).
ALTER TABLE users
    ADD COLUMN messages_prime boolean NOT NULL DEFAULT false,
    ADD COLUMN message_price bigint NOT NULL DEFAULT 0 CHECK (message_price BETWEEN 0 AND 1000000);

-- The paid posts people opened. The posts live in the post service, so
-- post_id refers to nothing here.
CREATE TABLE post_purchases (
    post_id uuid NOT NULL,
    buyer_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    author_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    price bigint NOT NULL CHECK (price > 0),
    bought_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (buyer_id, post_id)
);

-- The messages paid for, kept so that the votes go back when the message
-- is not sent.
CREATE TABLE message_payments (
    id uuid PRIMARY KEY,
    sender_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    recipient_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    price bigint NOT NULL CHECK (price > 0),
    tax bigint NOT NULL CHECK (tax BETWEEN 0 AND price)
);

ALTER TABLE vote_operations
    DROP CONSTRAINT vote_operations_kind_check,
    ADD CONSTRAINT vote_operations_kind_check
        CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake', 'rocket', 'bonus', 'denomination', 'fee', 'tax', 'rating', 'prime',
                        'gift_upgrade', 'gift_transfer', 'gift_sale', 'gift_fee', 'gift_bid', 'gift_refund', 'mine', 'mine_chest', 'post', 'message'));

-- +goose Down
DELETE FROM vote_operations
WHERE kind IN ('post', 'message');

ALTER TABLE vote_operations
    DROP CONSTRAINT vote_operations_kind_check,
    ADD CONSTRAINT vote_operations_kind_check
        CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake', 'rocket', 'bonus', 'denomination', 'fee', 'tax', 'rating', 'prime',
                        'gift_upgrade', 'gift_transfer', 'gift_sale', 'gift_fee', 'gift_bid', 'gift_refund', 'mine', 'mine_chest'));

DROP TABLE message_payments;
DROP TABLE post_purchases;

ALTER TABLE users
    DROP COLUMN message_price,
    DROP COLUMN messages_prime;
