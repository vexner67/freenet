-- +goose Up
-- The hash of the refresh token the current one replaced: it stays good
-- until the current one is used, so a client that never got the answer of
-- its refresh (a page left while it was on the way) keeps its session.
-- The default serves the previous version of auth, which does not write
-- the column, and the sessions that exist: they have no previous token.
ALTER TABLE sessions
    ADD COLUMN previous_refresh_token_hash text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE sessions
    DROP COLUMN previous_refresh_token_hash;
