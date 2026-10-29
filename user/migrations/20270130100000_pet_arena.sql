-- +goose Up
-- The market of pets: owned_since is when the owner got the pet (NULL —
-- when it was born), sale_price the votes it is up for, 0 — not for
-- sale; boss_at is when it last hit the boss of the week.
ALTER TABLE pets
    ADD COLUMN owned_since timestamptz,
    ADD COLUMN sale_price bigint NOT NULL DEFAULT 0 CHECK (sale_price >= 0),
    ADD COLUMN boss_at timestamptz;

CREATE INDEX pets_on_sale_idx ON pets (sale_price, id) WHERE sale_price > 0;

-- The boss of each week (user.PetBoss) and the tournament of each week
-- (user.PetTournament), as their states: the hits it took, the entrants,
-- the bets and the rounds.
CREATE TABLE pet_bosses (
    week date PRIMARY KEY,
    state jsonb NOT NULL CHECK (jsonb_typeof(state) = 'object')
);

CREATE TABLE pet_tournaments (
    week date PRIMARY KEY,
    state jsonb NOT NULL CHECK (jsonb_typeof(state) = 'object')
);

-- A season of the rating once closed: its best pets with their prizes.
CREATE TABLE pet_seasons (
    season date PRIMARY KEY,
    places jsonb NOT NULL CHECK (jsonb_typeof(places) = 'array'),
    closed_at timestamptz NOT NULL
);

-- The tasks of the day of each person (user.PetQuests) and the badges
-- they earned.
CREATE TABLE pet_quests (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    state jsonb NOT NULL CHECK (jsonb_typeof(state) = 'object')
);

CREATE TABLE pet_achievements (
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    achievement text NOT NULL,
    got_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, achievement)
);

-- +goose Down
DROP TABLE pet_achievements;
DROP TABLE pet_quests;
DROP TABLE pet_seasons;
DROP TABLE pet_tournaments;
DROP TABLE pet_bosses;

DROP INDEX pets_on_sale_idx;

ALTER TABLE pets
    DROP COLUMN boss_at,
    DROP COLUMN sale_price,
    DROP COLUMN owned_since;
