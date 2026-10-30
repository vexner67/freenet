-- +goose Up
ALTER TABLE vote_bonuses ADD COLUMN streak integer NOT NULL DEFAULT 1 CHECK (streak >= 1);

-- +goose Down
ALTER TABLE vote_bonuses DROP COLUMN streak;
