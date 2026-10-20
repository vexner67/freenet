-- +goose Up
-- The bets on events: the administrators ask a question with outcomes,
-- people stake votes on them, and the stakes go to those who guessed when
-- the event is resolved (winner — the index of the outcome), or back to
-- everyone when it is cancelled. closes_at is when the stakes close by
-- themselves, NULL for when the administrators close them.
CREATE TABLE bet_events (
    id uuid PRIMARY KEY,
    question text NOT NULL CHECK (char_length(question) BETWEEN 1 AND 200),
    outcomes text[] NOT NULL CHECK (cardinality(outcomes) BETWEEN 2 AND 10),
    state text NOT NULL DEFAULT 'open' CHECK (state IN ('open', 'closed', 'resolved', 'cancelled')),
    closes_at timestamptz,
    winner integer CHECK (winner >= 0 AND winner < cardinality(outcomes)),
    CONSTRAINT bet_events_winner CHECK ((state = 'resolved') = (winner IS NOT NULL))
);

-- One row per person and outcome: more stakes on it add up. payout is what
-- the row paid back once the event was over.
CREATE TABLE bet_stakes (
    event_id uuid NOT NULL REFERENCES bet_events (id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    outcome integer NOT NULL CHECK (outcome >= 0 AND outcome < 10),
    amount bigint NOT NULL CHECK (amount > 0),
    payout bigint NOT NULL DEFAULT 0 CHECK (payout >= 0),
    PRIMARY KEY (event_id, user_id, outcome)
);

CREATE INDEX bet_stakes_user_id_idx ON bet_stakes (user_id);

-- +goose Down
DROP TABLE bet_stakes;
DROP TABLE bet_events;
