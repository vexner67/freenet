-- +goose Up
-- A person keeps up to ten pets (user.MaxPets): a pet has its own ID.
ALTER TABLE pets ADD COLUMN id uuid NOT NULL DEFAULT uuidv7();
ALTER TABLE pets ALTER COLUMN id DROP DEFAULT;

ALTER TABLE pets DROP CONSTRAINT pets_pkey;
ALTER TABLE pets ADD PRIMARY KEY (id);

CREATE INDEX pets_user_idx ON pets (user_id, id);

DROP INDEX pets_rating_idx;
CREATE INDEX pets_rating_idx ON pets (rating DESC, id);

-- +goose Down
-- A person keeps their oldest pet only.
DELETE FROM pets AS p
WHERE EXISTS (
    SELECT 1
    FROM pets AS older
    WHERE older.user_id = p.user_id
        AND older.id < p.id
);

DROP INDEX pets_rating_idx;
CREATE INDEX pets_rating_idx ON pets (rating DESC, user_id);

DROP INDEX pets_user_idx;

ALTER TABLE pets DROP CONSTRAINT pets_pkey;
ALTER TABLE pets ADD PRIMARY KEY (user_id);
ALTER TABLE pets DROP COLUMN id;
