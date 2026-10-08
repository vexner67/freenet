-- +goose Up
-- The photo itself is stored by the media service.
ALTER TABLE clubs ADD COLUMN avatar_id uuid;

-- The clubs a user runs and keeps in the left menu ("Добавить в левое
-- меню" of openvk); the service keeps at most ten.
ALTER TABLE club_members ADD COLUMN pinned boolean NOT NULL DEFAULT false;

-- Club search: every word of the query is matched with LIKE '%word%'
-- against this expression, which a trigram index serves.
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX clubs_search_idx ON clubs
    USING gin (lower(name || ' ' || COALESCE(address, '')) gin_trgm_ops);

-- +goose Down
DROP INDEX clubs_search_idx;
ALTER TABLE club_members DROP COLUMN pinned;
ALTER TABLE clubs DROP COLUMN avatar_id;
