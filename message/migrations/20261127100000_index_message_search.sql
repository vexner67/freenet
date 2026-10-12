-- +goose Up
-- The search is a substring, whatever the case, of what a message says
-- itself or in the messages it carries on. message_words puts all of that
-- into one text, so that one trigram index finds both; with the two people
-- in the same index a search reads only what one person's messages say
-- about the query, however many messages they have.
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS btree_gin;

-- +goose StatementBegin
CREATE FUNCTION message_words(said text, forwarded jsonb) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE
RETURN said || coalesce((SELECT string_agg(E'\n' || (f->>'text'), '') FROM jsonb_array_elements(forwarded) AS f), '');
-- +goose StatementEnd

CREATE INDEX messages_search_idx ON messages USING gin (sender_id, recipient_id, message_words(text, forwarded) gin_trgm_ops);

-- +goose Down
DROP INDEX messages_search_idx;
DROP FUNCTION message_words(text, jsonb);
