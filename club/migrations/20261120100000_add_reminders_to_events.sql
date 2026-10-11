-- +goose Up
-- When the members of an event were told it starts soon ("В ближайшее
-- время состоится событие" of openvk); NULL until then and again after
-- its start is changed. The index is what the reminder looks through.
ALTER TABLE clubs
    ADD COLUMN reminded_at timestamptz;

-- Events that have started are not reminded of.
UPDATE clubs SET reminded_at = now() WHERE kind = 'event' AND starts_at <= now();

CREATE INDEX clubs_unreminded_events_idx ON clubs (starts_at) WHERE kind = 'event' AND reminded_at IS NULL;

-- +goose Down
DROP INDEX clubs_unreminded_events_idx;

ALTER TABLE clubs
    DROP COLUMN reminded_at;
