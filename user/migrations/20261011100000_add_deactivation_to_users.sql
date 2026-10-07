-- +goose Up
-- A page its owner deleted ("Удаление страницы" on openvk): hidden, and
-- restorable until purge_at, when its data is erased for good.
ALTER TABLE users
    ADD COLUMN deactivated_at      timestamptz,
    ADD COLUMN purge_at            timestamptz,
    ADD COLUMN deactivation_reason text NOT NULL DEFAULT '',
    ADD CONSTRAINT users_deactivation_check CHECK ((deactivated_at IS NULL) = (purge_at IS NULL));

-- The purge looks for the pages that are due.
CREATE INDEX users_purge_at_idx ON users (purge_at) WHERE purge_at IS NOT NULL;

-- +goose Down
DROP INDEX users_purge_at_idx;

ALTER TABLE users
    DROP CONSTRAINT users_deactivation_check,
    DROP COLUMN deactivation_reason,
    DROP COLUMN purge_at,
    DROP COLUMN deactivated_at;
