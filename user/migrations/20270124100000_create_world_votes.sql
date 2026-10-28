-- +goose Up
-- The operations of votes the world service asked for: ChargeVotes takes
-- votes (amount below zero), PayVotes pays them. A repeat with the same
-- op_id changes nothing.
CREATE TABLE world_votes (
    op_id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    amount bigint NOT NULL CHECK (amount <> 0),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE world_votes;
