-- +goose Up
-- Cases give eight more kinds of pets (user.PetKinds), and a pet wears
-- clothes: wardrobe — what was bought for it, outfit — what it wears.
ALTER TABLE pets
    DROP CONSTRAINT pets_kind_check,
    ADD CONSTRAINT pets_kind_check
        CHECK (kind IN ('cat', 'dog', 'hamster', 'dragon', 'rabbit', 'turtle', 'fox', 'owl', 'wolf', 'panda', 'unicorn', 'phoenix')),
    ADD COLUMN wardrobe text[] NOT NULL DEFAULT '{}',
    ADD COLUMN outfit text[] NOT NULL DEFAULT '{}',
    ADD CONSTRAINT pets_outfit_check CHECK (outfit <@ wardrobe);

-- +goose Down
-- The kinds of cases go with their pets, and the clothes with them.
DELETE FROM pets
WHERE kind NOT IN ('cat', 'dog', 'hamster', 'dragon');

ALTER TABLE pets
    DROP CONSTRAINT pets_outfit_check,
    DROP COLUMN outfit,
    DROP COLUMN wardrobe,
    DROP CONSTRAINT pets_kind_check,
    ADD CONSTRAINT pets_kind_check CHECK (kind IN ('cat', 'dog', 'hamster', 'dragon'));
