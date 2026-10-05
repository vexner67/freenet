-- +goose Up
-- The photo itself is stored by the media service.
ALTER TABLE users ADD COLUMN avatar_id uuid;

-- People search: every word of the query is matched with LIKE '%word%'
-- against this expression, which a trigram index serves.
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX users_search_idx ON users
    USING gin (lower(nickname || ' ' || name || ' ' || surname) gin_trgm_ops);

-- +goose Down
DROP INDEX users_search_idx;
ALTER TABLE users DROP COLUMN avatar_id;
