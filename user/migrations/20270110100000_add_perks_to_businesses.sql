-- +goose Up
-- The improvements of a business: the tier of each bought one, by name
-- (user.BusinessPerks), from 1 to 3.
ALTER TABLE businesses
    ADD COLUMN perks jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(perks) = 'object');

-- +goose Down
ALTER TABLE businesses
    DROP COLUMN perks;
