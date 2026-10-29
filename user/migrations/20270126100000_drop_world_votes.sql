-- +goose Up
-- The states of «Мир» are no more: nobody asks for votes of the world, and
-- the world has nothing of a page to erase. The operations of votes stay
-- in vote_operations, the history of the wallets.
DROP TABLE world_votes;
DELETE FROM outbox WHERE stream = 'world';

-- +goose Down
CREATE TABLE world_votes (
    op_id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    amount bigint NOT NULL CHECK (amount <> 0),
    created_at timestamptz NOT NULL DEFAULT now()
);
