-- +goose Up
-- The links of a page lead to people from the same town or with the same
-- favourite: these searches read every user until now. A town is matched
-- whole, whatever the case, and paged by id; a favourite is matched
-- anywhere in its field, which a trigram index serves, as the names are.
CREATE INDEX users_hometown_idx ON users (lower(hometown), id);
CREATE INDEX users_city_idx ON users (lower(city), id);
CREATE INDEX users_fav_films_idx ON users USING gin (lower(fav_films) gin_trgm_ops);
CREATE INDEX users_fav_shows_idx ON users USING gin (lower(fav_shows) gin_trgm_ops);
CREATE INDEX users_fav_books_idx ON users USING gin (lower(fav_books) gin_trgm_ops);

-- +goose Down
DROP INDEX users_fav_books_idx;
DROP INDEX users_fav_shows_idx;
DROP INDEX users_fav_films_idx;
DROP INDEX users_city_idx;
DROP INDEX users_hometown_idx;
