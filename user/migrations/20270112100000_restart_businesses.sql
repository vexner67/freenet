-- +goose Up
-- The businesses start again as a game of supplies, offers and workers:
-- each owner gets back from the treasury what he put into a business and
-- has not taken out, and the whole votes of its till, and the businesses
-- and their old settings go.
WITH refunds AS (
    SELECT user_id,
           sum(greatest(invested - collected, 0) + greatest(cash, 0) / 100) AS votes
    FROM businesses
    GROUP BY user_id
    HAVING sum(greatest(invested - collected, 0) + greatest(cash, 0) / 100) > 0
),
paid AS (
    UPDATE users
    SET votes = users.votes + refunds.votes
    FROM refunds
    WHERE users.id = refunds.user_id
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

UPDATE business
SET settings = NULL;

-- A business of a page: its whole state as counted at counted_at
-- (user.BusinessState) — the till, the supplies, the workers, the orders,
-- the event and the last hours and notes. Only the kind is a column: the
-- business is read and written whole, in a transaction that locks it.
CREATE TABLE businesses (
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('kiosk', 'cafe', 'carwash', 'factory')),
    state jsonb NOT NULL CHECK (jsonb_typeof(state) = 'object'),
    PRIMARY KEY (user_id, kind)
);

-- +goose Down
-- The refunds stay paid: the old businesses are gone.
DROP TABLE businesses;

UPDATE business
SET settings = NULL;

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
    perks jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(perks) = 'object'),
    PRIMARY KEY (user_id, kind)
);
