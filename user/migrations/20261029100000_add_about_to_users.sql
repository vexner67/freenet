-- +goose Up
-- openvk's page: "Родной город", whom the relationship names, a birthday
-- without its year, "Контактная информация", "Личная информация" and
-- "Дополнительно" (up to 7 fields of the owner's naming). An erased
-- partner leaves the relationship naming nobody.
ALTER TABLE users
    ADD COLUMN hometown text NOT NULL DEFAULT '' CHECK (char_length(hometown) <= 64),
    ADD COLUMN relationship_partner_id uuid
        CONSTRAINT users_partner_fkey REFERENCES users (id) ON DELETE SET NULL
        CHECK (relationship_partner_id <> id),
    ADD COLUMN hide_birth_year boolean NOT NULL DEFAULT false,
    ADD COLUMN contact_email text NOT NULL DEFAULT '' CHECK (char_length(contact_email) <= 254),
    ADD COLUMN telegram text NOT NULL DEFAULT '' CHECK (char_length(telegram) <= 32),
    ADD COLUMN website text NOT NULL DEFAULT '' CHECK (char_length(website) <= 256),
    ADD COLUMN city text NOT NULL DEFAULT '' CHECK (char_length(city) <= 64),
    ADD COLUMN address text NOT NULL DEFAULT '' CHECK (char_length(address) <= 255),
    ADD COLUMN interests text NOT NULL DEFAULT '' CHECK (char_length(interests) <= 1000),
    ADD COLUMN fav_music text NOT NULL DEFAULT '' CHECK (char_length(fav_music) <= 1000),
    ADD COLUMN fav_films text NOT NULL DEFAULT '' CHECK (char_length(fav_films) <= 1000),
    ADD COLUMN fav_shows text NOT NULL DEFAULT '' CHECK (char_length(fav_shows) <= 1000),
    ADD COLUMN fav_books text NOT NULL DEFAULT '' CHECK (char_length(fav_books) <= 1000),
    ADD COLUMN fav_quotes text NOT NULL DEFAULT '' CHECK (char_length(fav_quotes) <= 1000),
    ADD COLUMN fav_games text NOT NULL DEFAULT '' CHECK (char_length(fav_games) <= 1000),
    ADD COLUMN about text NOT NULL DEFAULT '' CHECK (char_length(about) <= 1000),
    ADD COLUMN extra_fields jsonb NOT NULL DEFAULT '[]'
        CHECK (jsonb_typeof(extra_fields) = 'array' AND jsonb_array_length(extra_fields) <= 7);

-- ON DELETE SET NULL looks the partners of an erased user up.
CREATE INDEX users_partner_idx ON users (relationship_partner_id) WHERE relationship_partner_id IS NOT NULL;

-- +goose Down
DROP INDEX users_partner_idx;
ALTER TABLE users
    DROP COLUMN extra_fields,
    DROP COLUMN about,
    DROP COLUMN fav_games,
    DROP COLUMN fav_quotes,
    DROP COLUMN fav_books,
    DROP COLUMN fav_shows,
    DROP COLUMN fav_films,
    DROP COLUMN fav_music,
    DROP COLUMN interests,
    DROP COLUMN address,
    DROP COLUMN city,
    DROP COLUMN website,
    DROP COLUMN telegram,
    DROP COLUMN contact_email,
    DROP COLUMN hide_birth_year,
    DROP COLUMN relationship_partner_id,
    DROP COLUMN hometown;
