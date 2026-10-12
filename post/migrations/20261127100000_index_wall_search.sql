-- +goose Up
-- "Поиск" on a wall is a substring of the text, whatever the case: trigrams
-- find it, and with the wall in the same index a search reads only what one
-- wall says about the query, however many posts the wall has.
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS btree_gin;

CREATE INDEX posts_wall_search_idx ON posts USING gin (wall_id, text gin_trgm_ops);

-- +goose Down
DROP INDEX posts_wall_search_idx;
