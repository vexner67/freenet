-- +goose Up
-- The worlds of «Мир» (world.World): made of a seed on a grid, their map
-- generated in the background (lease_until keeps other replicas off it),
-- then opened by the administrators. One world at most is open.
CREATE TABLE worlds (
    id uuid PRIMARY KEY,
    seed bigint NOT NULL,
    grid integer NOT NULL CHECK (grid IN (1024, 2048, 4096)),
    status text NOT NULL CHECK (status IN ('generating', 'ready', 'open', 'ended', 'failed')),
    created_at timestamptz NOT NULL,
    opened_at timestamptz,
    ended_at timestamptz,
    lease_until timestamptz
);

CREATE UNIQUE INDEX worlds_open_idx ON worlds ((true)) WHERE status = 'open';

-- The map of a world as it was made: the provinces (world.Atlas) and the
-- province of every cell, gzipped little-endian uint16, for the site.
CREATE TABLE world_maps (
    world_id uuid PRIMARY KEY REFERENCES worlds (id) ON DELETE CASCADE,
    atlas jsonb NOT NULL,
    raster bytea NOT NULL
);

-- The pictures of the map, 256×256 PNG, zoom by zoom.
CREATE TABLE world_tiles (
    world_id uuid NOT NULL REFERENCES worlds (id) ON DELETE CASCADE,
    z smallint NOT NULL CHECK (z >= 0),
    x smallint NOT NULL CHECK (x >= 0),
    y smallint NOT NULL CHECK (y >= 0),
    png bytea NOT NULL,
    PRIMARY KEY (world_id, z, x, y)
);

-- What people made of an open world (world.Game): the states, their land,
-- citizens, applications and colonies, one aggregate every change locks.
CREATE TABLE world_games (
    world_id uuid PRIMARY KEY REFERENCES worlds (id) ON DELETE CASCADE,
    game jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- The news of the worlds (world.Event).
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

-- The rules of «Мир» (world.Rules) the administrators set; without the
-- row the defaults hold.
CREATE TABLE world_rules (
    id boolean PRIMARY KEY DEFAULT true CHECK (id),
    rules jsonb NOT NULL
);

-- +goose Down
DROP TABLE world_rules;
DROP TABLE world_events;
DROP TABLE world_games;
DROP TABLE world_tiles;
DROP TABLE world_maps;
DROP TABLE worlds;
