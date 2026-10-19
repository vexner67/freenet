-- +goose Up
-- A club the administrators of the site confirmed wears the checkmark by
-- its name, as a verified page does. Only they set it, through the
-- gateway; the admin panel lists the verified clubs by the partial index.
ALTER TABLE clubs
    ADD COLUMN verified boolean NOT NULL DEFAULT false;

CREATE INDEX clubs_verified_idx ON clubs (id DESC) WHERE verified;

-- +goose Down
DROP INDEX clubs_verified_idx;

ALTER TABLE clubs
    DROP COLUMN verified;
