-- +goose Up
-- Close the bank in one transaction. Keep the legacy schema so previous
-- binaries can still start during a rollback.
LOCK TABLE bank_accounts IN SHARE ROW EXCLUSIVE MODE;

-- Apply the same daily rounding, earning limit, balance cap and 3650-day
-- horizon as the old bank. The wallet accepts whole votes.
WITH RECURSIVE
rates AS (
    SELECT coalesce((settings ->> 'depositRate')::integer, 20) AS rate,
           coalesce((settings ->> 'depositMost')::bigint, 10000) * 100 AS earning_limit
    FROM bank
),
accrued (user_id, amount, days_left) AS (
    SELECT user_id, deposit,
           greatest(0, least(3650, floor(extract(epoch FROM (now() - deposit_at)) / 86400)))::integer
    FROM bank_accounts
    WHERE deposit > 0
    UNION ALL
    SELECT user_id,
           least(100000000000000000, amount + div(least(amount, earning_limit)::numeric * rate, 10000))::bigint,
           days_left - 1
    FROM accrued CROSS JOIN rates
    WHERE days_left > 0
),
refunds AS (
    SELECT user_id, amount / 100 AS votes
    FROM accrued
    WHERE days_left = 0 AND amount >= 100
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
    SELECT uuidv7(), id, 'deposit', amount, balance, 'closed'
    FROM paid
    RETURNING amount
)
UPDATE treasury
SET votes = votes - coalesce((SELECT sum(amount) FROM noted), 0);

UPDATE bank_accounts
SET deposit = 0, deposit_at = DEFAULT, debt = 0, debt_at = DEFAULT,
    borrowed = 0, due_at = DEFAULT
WHERE deposit > 0 OR debt > 0;

-- +goose Down
-- The refunds stay paid and the loans stay closed.
SELECT 1;
