-- +goose Up
-- A pet defends in a stance and tires in fights in a row: streak are the
-- fights in a row, the last at streak_at.
ALTER TABLE pets
    ADD COLUMN stance text NOT NULL DEFAULT 'defense' CHECK (stance IN ('attack', 'defense', 'agility')),
    ADD COLUMN streak smallint NOT NULL DEFAULT 0 CHECK (streak >= 0),
    ADD COLUMN streak_at timestamptz;

-- What a person keeps of the cases (user.PetKeeper): the shards, the cases
-- since the last legendary pet, those of the day cases_on and the pet a
-- case gave, not taken yet.
CREATE TABLE pet_keepers (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    shards integer NOT NULL DEFAULT 0 CHECK (shards >= 0),
    pity integer NOT NULL DEFAULT 0 CHECK (pity >= 0),
    cases_on date,
    cases integer NOT NULL DEFAULT 0 CHECK (cases >= 0),
    drop_kind text,
    drop_case text CHECK (drop_case IN ('bronze', 'silver', 'gold')),
    CHECK ((drop_kind IS NULL) = (drop_case IS NULL))
);

-- The fights led a round after another (user.PetBattle), one a pet: the
-- state is the combatants, the acts so far and what the fight brings.
CREATE TABLE pet_battles (
    pet_id uuid PRIMARY KEY REFERENCES pets (id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    state jsonb NOT NULL CHECK (jsonb_typeof(state) = 'object'),
    started_at timestamptz NOT NULL
);

CREATE INDEX pet_battles_user_idx ON pet_battles (user_id);

-- +goose Down
DROP TABLE pet_battles;
DROP TABLE pet_keepers;

ALTER TABLE pets
    DROP COLUMN streak_at,
    DROP COLUMN streak,
    DROP COLUMN stance;
