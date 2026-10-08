-- +goose Up
-- "Сайт" of openvk: an http or https link, '' for none.
ALTER TABLE clubs ADD COLUMN website text NOT NULL DEFAULT ''
    CHECK (char_length(website) <= 256);

-- What the page shows of those who run the club, openvk's "Список админов".
ALTER TABLE clubs ADD COLUMN admins_display text NOT NULL DEFAULT 'creator'
    CHECK (admins_display IN ('creator', 'all', 'none'));

-- +goose Down
ALTER TABLE clubs DROP COLUMN admins_display;
ALTER TABLE clubs DROP COLUMN website;
