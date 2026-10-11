-- +goose Up
-- When and where from a session was last refreshed, for its owner's list
-- of sessions. The defaults serve the previous version of auth, which
-- does not write the columns, and the sessions that exist: those count as
-- used now, from an unknown device.
ALTER TABLE sessions
    ADD COLUMN last_used_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN ip           text        NOT NULL DEFAULT '',
    ADD COLUMN user_agent   text        NOT NULL DEFAULT '';

-- The sessions of a user, the latest used first.
CREATE INDEX sessions_user_id_last_used_at_idx ON sessions (user_id, last_used_at DESC, id DESC);

-- +goose Down
DROP INDEX sessions_user_id_last_used_at_idx;

ALTER TABLE sessions
    DROP COLUMN last_used_at,
    DROP COLUMN ip,
    DROP COLUMN user_agent;
