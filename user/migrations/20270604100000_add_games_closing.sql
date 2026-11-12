-- +goose Up
CREATE TABLE games_closure (
    only_row boolean PRIMARY KEY DEFAULT true CHECK (only_row),
    frozen_at timestamptz NOT NULL
);

CREATE TABLE games_closing (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    amounts jsonb NOT NULL CHECK (jsonb_typeof(amounts) = 'object'),
    total bigint NOT NULL CHECK (total > 0),
    paid_at timestamptz
);

ALTER TABLE vote_operations
    DROP CONSTRAINT vote_operations_kind_check,
    ADD CONSTRAINT vote_operations_kind_check
        CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake', 'rocket', 'bonus', 'denomination', 'fee', 'tax', 'rating', 'prime',
                        'gift_upgrade', 'gift_transfer', 'gift_sale', 'gift_fee', 'gift_bid', 'gift_refund', 'mine', 'mine_chest', 'post', 'message',
                        'business', 'deposit', 'loan', 'poker', 'pet', 'world', 'closing'));

-- +goose Down
-- A settlement that was frozen must stay recorded to prevent double payment.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM games_closure) THEN
        RAISE EXCEPTION 'cannot remove a recorded game settlement';
    END IF;
END;
$$;
-- +goose StatementEnd
ALTER TABLE vote_operations
    DROP CONSTRAINT vote_operations_kind_check,
    ADD CONSTRAINT vote_operations_kind_check
        CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake', 'rocket', 'bonus', 'denomination', 'fee', 'tax', 'rating', 'prime',
                        'gift_upgrade', 'gift_transfer', 'gift_sale', 'gift_fee', 'gift_bid', 'gift_refund', 'mine', 'mine_chest', 'post', 'message',
                        'business', 'deposit', 'loan', 'poker', 'pet', 'world'));
DROP TABLE games_closing;
DROP TABLE games_closure;
