-- +goose Up
-- The bank of the site, one row: its settings (NULL — user.DefaultBank).
CREATE TABLE bank (
    only_row boolean PRIMARY KEY DEFAULT true CHECK (only_row),
    settings jsonb CHECK (settings IS NULL OR jsonb_typeof(settings) = 'object')
);

INSERT INTO bank DEFAULT VALUES;

-- The deposit and the loan of a page (user.BankAccount): the deposit and
-- the debt in hundredths of a vote, each with the time its interest is
-- counted up to, what was borrowed and when it is due.
CREATE TABLE bank_accounts (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    deposit bigint NOT NULL DEFAULT 0 CHECK (deposit >= 0),
    deposit_at timestamptz NOT NULL DEFAULT '0001-01-01 00:00:00+00',
    debt bigint NOT NULL DEFAULT 0 CHECK (debt >= 0),
    debt_at timestamptz NOT NULL DEFAULT '0001-01-01 00:00:00+00',
    borrowed bigint NOT NULL DEFAULT 0 CHECK (borrowed >= 0),
    due_at timestamptz NOT NULL DEFAULT '0001-01-01 00:00:00+00',
    CHECK ((debt = 0) = (borrowed = 0))
);

-- The loans past their term, which the site collects.
CREATE INDEX bank_accounts_due_idx ON bank_accounts (due_at) WHERE debt > 0;

-- Deposits and loans are lines of the statement.
ALTER TABLE vote_operations
    DROP CONSTRAINT vote_operations_kind_check,
    ADD CONSTRAINT vote_operations_kind_check
        CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake', 'rocket', 'bonus', 'denomination', 'fee', 'tax', 'rating', 'prime',
                        'gift_upgrade', 'gift_transfer', 'gift_sale', 'gift_fee', 'gift_bid', 'gift_refund', 'mine', 'mine_chest', 'post', 'message',
                        'business', 'deposit', 'loan'));

-- +goose Down
DELETE FROM vote_operations
WHERE kind IN ('deposit', 'loan');

ALTER TABLE vote_operations
    DROP CONSTRAINT vote_operations_kind_check,
    ADD CONSTRAINT vote_operations_kind_check
        CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake', 'rocket', 'bonus', 'denomination', 'fee', 'tax', 'rating', 'prime',
                        'gift_upgrade', 'gift_transfer', 'gift_sale', 'gift_fee', 'gift_bid', 'gift_refund', 'mine', 'mine_chest', 'post', 'message',
                        'business'));

DROP TABLE bank_accounts;
DROP TABLE bank;
