-- +goose Up
-- BIG 2Z, the pet named after churkez228, and the pets given once: a row
-- stays after the pet is released or sold, so it cannot be taken again.
ALTER TABLE pets
    DROP CONSTRAINT pets_kind_check,
    ADD CONSTRAINT pets_kind_check
        CHECK (kind IN ('cat', 'dog', 'hamster', 'dragon', 'rabbit', 'turtle', 'fox', 'owl', 'wolf', 'panda', 'unicorn', 'phoenix', 'big2z'));

CREATE TABLE pet_grants (
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind text NOT NULL,
    granted_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, kind)
);

-- +goose Down
DROP TABLE pet_grants;

DELETE FROM pets
WHERE kind = 'big2z';

ALTER TABLE pets
    DROP CONSTRAINT pets_kind_check,
    ADD CONSTRAINT pets_kind_check
        CHECK (kind IN ('cat', 'dog', 'hamster', 'dragon', 'rabbit', 'turtle', 'fox', 'owl', 'wolf', 'panda', 'unicorn', 'phoenix'));
