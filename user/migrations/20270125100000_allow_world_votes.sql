-- +goose Up
-- The votes of «Мир»: a state founded, its refund, a gift to a treasury.
ALTER TABLE vote_operations
    DROP CONSTRAINT vote_operations_kind_check,
    ADD CONSTRAINT vote_operations_kind_check
        CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake', 'rocket', 'bonus', 'denomination', 'fee', 'tax', 'rating', 'prime',
                        'gift_upgrade', 'gift_transfer', 'gift_sale', 'gift_fee', 'gift_bid', 'gift_refund', 'mine', 'mine_chest', 'post', 'message',
                        'business', 'deposit', 'loan', 'poker', 'pet', 'world'));

-- +goose Down
DELETE FROM vote_operations
WHERE kind = 'world';

ALTER TABLE vote_operations
    DROP CONSTRAINT vote_operations_kind_check,
    ADD CONSTRAINT vote_operations_kind_check
        CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake', 'rocket', 'bonus', 'denomination', 'fee', 'tax', 'rating', 'prime',
                        'gift_upgrade', 'gift_transfer', 'gift_sale', 'gift_fee', 'gift_bid', 'gift_refund', 'mine', 'mine_chest', 'post', 'message',
                        'business', 'deposit', 'loan', 'poker', 'pet'));
