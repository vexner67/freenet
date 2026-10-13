-- +goose Up
-- Visits live in presence; the column stayed, unread, for one version, so
-- that the one before could be rolled back to. That version is gone.
ALTER TABLE users
    DROP COLUMN last_seen_at;

-- +goose Down
ALTER TABLE users
    ADD COLUMN last_seen_at timestamptz;

UPDATE users u
SET last_seen_at = p.last_seen_at
FROM presence p
WHERE p.user_id = u.id;
