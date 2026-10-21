-- +goose Up
-- The treasury ("Казна"): the votes of the site that are no one's. Every
-- vote paid to a page comes out of it — bonuses, vouchers, wins — and
-- every vote spent goes back in, so votes appear only by the rules of the
-- code: 25 000 at the start and 1000 minted by each new member, once.
CREATE TABLE treasury (
    only_row boolean PRIMARY KEY DEFAULT true CHECK (only_row),
    votes bigint NOT NULL CHECK (votes >= 0),
    minted bigint NOT NULL DEFAULT 0 CHECK (minted >= 0)
);

INSERT INTO treasury (votes) VALUES (25000);

-- The members who have brought their 1000 into the treasury.
CREATE TABLE treasury_members (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    joined_at timestamptz NOT NULL DEFAULT now()
);

-- The day (of Moscow) each page took its last daily bonus.
CREATE TABLE vote_bonuses (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    day date NOT NULL
);

-- +goose Down
DROP TABLE vote_bonuses;

DROP TABLE treasury_members;

DROP TABLE treasury;
