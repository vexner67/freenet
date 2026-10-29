-- +goose Up
-- The states of people were removed from «Мир»: only the worlds and their
-- maps stay.
DROP TABLE world_rules;
DROP TABLE world_events;
DROP TABLE world_games;

-- +goose Down
CREATE TABLE world_games (
    world_id uuid PRIMARY KEY REFERENCES worlds (id) ON DELETE CASCADE,
    game jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE world_events (
    id uuid PRIMARY KEY,
    world_id uuid NOT NULL REFERENCES worlds (id) ON DELETE CASCADE,
    kind text NOT NULL,
    state_id uuid NOT NULL,
    user_id uuid,
    province integer,
    name text NOT NULL,
    role text NOT NULL DEFAULT '',
    at timestamptz NOT NULL
);

CREATE INDEX world_events_world_idx ON world_events (world_id, id DESC);
CREATE INDEX world_events_state_idx ON world_events (world_id, state_id, id DESC);

CREATE TABLE world_rules (
    id boolean PRIMARY KEY DEFAULT true CHECK (id),
    rules jsonb NOT NULL
);
