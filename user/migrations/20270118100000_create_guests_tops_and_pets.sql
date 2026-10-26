-- +goose Up
-- "Гости": the last visit of each guest to a page (user.Guest). A visit
-- of the same guest replaces the one before; the guests are read by the
-- owner, the last visit first.
CREATE TABLE page_visits (
    owner_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    guest_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    visited_at timestamptz NOT NULL,
    PRIMARY KEY (owner_id, guest_id),
    CHECK (owner_id <> guest_id)
);

CREATE INDEX page_visits_owner_idx ON page_visits (owner_id, visited_at DESC, guest_id DESC);
CREATE INDEX page_visits_guest_idx ON page_visits (guest_id);

-- When each owner last looked at their guests: newer visits are new.
CREATE TABLE guest_marks (
    owner_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    seen_at timestamptz NOT NULL
);

-- "Forbes Freenet" of "Рейтинги" reads the richest pages.
CREATE INDEX users_votes_idx ON users (votes DESC, id) WHERE votes > 0;

-- "Питомец", one row: the shop (NULL — user.DefaultPetShop) and how the
-- pets do — the votes spent on them and won by them.
CREATE TABLE pet_shop (
    only_row boolean PRIMARY KEY DEFAULT true CHECK (only_row),
    settings jsonb CHECK (settings IS NULL OR jsonb_typeof(settings) = 'object'),
    spent bigint NOT NULL DEFAULT 0 CHECK (spent >= 0),
    won bigint NOT NULL DEFAULT 0 CHECK (won >= 0)
);

INSERT INTO pet_shop DEFAULT VALUES;

-- The pets (user.Pet), one a person: satiety and joy are what they were
-- at counted_at; fights are those of the day fought_on.
CREATE TABLE pets (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('cat', 'dog', 'hamster', 'dragon')),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 20),
    born_at timestamptz NOT NULL,
    satiety smallint NOT NULL CHECK (satiety BETWEEN 0 AND 100),
    joy smallint NOT NULL CHECK (joy BETWEEN 0 AND 100),
    counted_at timestamptz NOT NULL,
    played_at timestamptz,
    experience bigint NOT NULL DEFAULT 0 CHECK (experience >= 0),
    rating integer NOT NULL,
    wins integer NOT NULL DEFAULT 0 CHECK (wins >= 0),
    losses integer NOT NULL DEFAULT 0 CHECK (losses >= 0),
    fought_on date,
    fights integer NOT NULL DEFAULT 0 CHECK (fights >= 0)
);

CREATE INDEX pets_rating_idx ON pets (rating DESC, user_id);

-- The fights of pets (user.PetFight): who set whose pet on whose, the pets
-- as they were and how it went — the hits, won and the rating moved.
CREATE TABLE pet_fights (
    id uuid PRIMARY KEY,
    attacker_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    defender_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    bout jsonb NOT NULL CHECK (jsonb_typeof(bout) = 'object'),
    fought_at timestamptz NOT NULL
);

CREATE INDEX pet_fights_attacker_idx ON pet_fights (attacker_id, id DESC);
CREATE INDEX pet_fights_defender_idx ON pet_fights (defender_id, id DESC);

ALTER TABLE vote_operations
    DROP CONSTRAINT vote_operations_kind_check,
    ADD CONSTRAINT vote_operations_kind_check
        CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake', 'rocket', 'bonus', 'denomination', 'fee', 'tax', 'rating', 'prime',
                        'gift_upgrade', 'gift_transfer', 'gift_sale', 'gift_fee', 'gift_bid', 'gift_refund', 'mine', 'mine_chest', 'post', 'message',
                        'business', 'deposit', 'loan', 'poker', 'pet'));

-- The weekly boards sum the statement by kind over a week of IDs.
CREATE INDEX vote_operations_kind_idx ON vote_operations (kind, id);

-- +goose Down
DROP INDEX vote_operations_kind_idx;

DELETE FROM vote_operations
WHERE kind = 'pet';

ALTER TABLE vote_operations
    DROP CONSTRAINT vote_operations_kind_check,
    ADD CONSTRAINT vote_operations_kind_check
        CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake', 'rocket', 'bonus', 'denomination', 'fee', 'tax', 'rating', 'prime',
                        'gift_upgrade', 'gift_transfer', 'gift_sale', 'gift_fee', 'gift_bid', 'gift_refund', 'mine', 'mine_chest', 'post', 'message',
                        'business', 'deposit', 'loan', 'poker'));

DROP TABLE pet_fights;
DROP TABLE pets;
DROP TABLE pet_shop;
DROP INDEX users_votes_idx;
DROP TABLE guest_marks;
DROP TABLE page_visits;
