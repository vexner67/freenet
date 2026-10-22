-- +goose Up
-- The treasury has no bottom now: it pays whatever the rules of the site
-- say, so what it holds is only how much more it has taken than paid since
-- it was made, below zero when it paid more. Members no longer mint into
-- it.
ALTER TABLE treasury
    DROP CONSTRAINT treasury_votes_check,
    DROP COLUMN minted;

DROP TABLE treasury_members;

-- +goose Down
CREATE TABLE treasury_members (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    joined_at timestamptz NOT NULL DEFAULT now()
);

UPDATE treasury SET votes = greatest(votes, 0);

ALTER TABLE treasury
    ADD COLUMN minted bigint NOT NULL DEFAULT 0 CHECK (minted >= 0),
    ADD CONSTRAINT treasury_votes_check CHECK (votes >= 0);
