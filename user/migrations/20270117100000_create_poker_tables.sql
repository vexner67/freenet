-- +goose Up
-- The tables of "Покер" (user.HoldemTable): Texas Hold'em of people. The
-- table is all in state (seats, cards, bets — user.HoldemState as JSON);
-- the columns beside it are what the lobby, the quick game and the sweep
-- look for: who sits (player_ids) and is invited (invited_ids), the seats
-- taken and when the table has to move on by itself (due_at). An empty
-- table is deleted.
CREATE TABLE poker_tables (
    id uuid PRIMARY KEY,
    private boolean NOT NULL,
    big_blind bigint NOT NULL CHECK (big_blind >= 2),
    seats smallint NOT NULL CHECK (seats BETWEEN 2 AND 9),
    players smallint NOT NULL CHECK (players BETWEEN 0 AND seats),
    player_ids uuid[] NOT NULL DEFAULT '{}',
    invited_ids uuid[] NOT NULL DEFAULT '{}',
    due_at timestamptz,
    state jsonb NOT NULL CHECK (jsonb_typeof(state) = 'object'),
    created_at timestamptz NOT NULL
);

CREATE INDEX poker_tables_open_idx ON poker_tables (big_blind, players DESC) WHERE NOT private;
CREATE INDEX poker_tables_due_idx ON poker_tables (due_at) WHERE due_at IS NOT NULL;
CREATE INDEX poker_tables_player_ids_idx ON poker_tables USING gin (player_ids);
CREATE INDEX poker_tables_invited_ids_idx ON poker_tables USING gin (invited_ids);

-- +goose Down
DROP TABLE poker_tables;
