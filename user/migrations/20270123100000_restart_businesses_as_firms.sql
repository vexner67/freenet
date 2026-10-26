-- +goose Up
-- The businesses start again as an economy of enterprises: each owner gets
-- back from the treasury twice what he put into buying and upgrading his
-- businesses, bankrupt ones too, and the whole votes left in their tills;
-- debts are forgiven. The businesses and their settings go.
WITH refunds AS (
    SELECT user_id,
           sum(2 * (state ->> 'invested')::bigint
               + greatest(floor((state ->> 'cash')::float8), 0)::bigint / 100) AS votes
    FROM businesses
    GROUP BY user_id
),
paid AS (
    UPDATE users
    SET votes = users.votes + refunds.votes
    FROM refunds
    WHERE users.id = refunds.user_id
      AND refunds.votes > 0
    RETURNING users.id, refunds.votes AS amount, users.votes AS balance
),
noted AS (
    INSERT INTO vote_operations (id, user_id, kind, amount, balance, note)
    SELECT uuidv7(), id, 'business', amount, balance, 'refund'
    FROM paid
    RETURNING amount
)
UPDATE treasury
SET votes = votes - coalesce((SELECT sum(amount) FROM noted), 0);

DROP TABLE businesses;
DROP TABLE business;

-- The economy of the enterprises (user.City) in one row: the rules the
-- administrators set (NULL — the defaults of the code,
-- user.DefaultFirmRules) and the whole city — the enterprises, their
-- shareholders, the orders of the exchange, the offers of shares and what
-- changed hands lately (NULL — none yet). The city is read and written
-- whole, in a transaction that locks the row: one change at a time.
CREATE TABLE firm_city (
    only_row boolean PRIMARY KEY DEFAULT true CHECK (only_row),
    settings jsonb CHECK (settings IS NULL OR jsonb_typeof(settings) = 'object'),
    state jsonb CHECK (state IS NULL OR jsonb_typeof(state) = 'object')
);

INSERT INTO firm_city DEFAULT VALUES;

-- +goose Down
-- The refunds stay paid: the old businesses are gone.
DROP TABLE firm_city;

CREATE TABLE business (
    only_row boolean PRIMARY KEY DEFAULT true CHECK (only_row),
    settings jsonb CHECK (settings IS NULL OR jsonb_typeof(settings) = 'object'),
    spent bigint NOT NULL DEFAULT 0 CHECK (spent >= 0),
    paid bigint NOT NULL DEFAULT 0 CHECK (paid >= 0)
);

INSERT INTO business DEFAULT VALUES;

CREATE TABLE businesses (
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('kiosk', 'cafe', 'carwash', 'factory')),
    state jsonb NOT NULL CHECK (jsonb_typeof(state) = 'object'),
    PRIMARY KEY (user_id, kind)
);
