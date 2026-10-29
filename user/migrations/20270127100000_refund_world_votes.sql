-- +goose Up
-- The states of «Мир» were abolished: everyone gets back from the treasury
-- what he paid for them and did not get back — founding, donations —
-- as the businesses did when they started again.
WITH refunds AS (
    SELECT user_id, -sum(amount) AS votes
    FROM vote_operations
    WHERE kind = 'world'
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
    SELECT uuidv7(), id, 'world', amount, balance, 'closed'
    FROM paid
    RETURNING amount
)
UPDATE treasury
SET votes = votes - coalesce((SELECT sum(amount) FROM noted), 0);

-- +goose Down
-- The refunds stay paid: the states are gone.
SELECT 1;
