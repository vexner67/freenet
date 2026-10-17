-- +goose Up
-- Events ("Встречи") are no more. Those there are go with everything of
-- theirs: post erases their walls, topics and albums (and has media erase
-- their photos), notification what they caused, both told through the
-- outbox; their members and join requests go with them by the foreign
-- keys.
INSERT INTO outbox (stream, kind, payload)
SELECT 'post', 'purge_club', uuid_send(id)
FROM clubs
WHERE kind = 'event'
ORDER BY id;

INSERT INTO outbox (stream, kind, payload)
SELECT 'notification', 'purge_club', uuid_send(id)
FROM clubs
WHERE kind = 'event'
ORDER BY id;

DELETE FROM clubs WHERE kind = 'event';

DROP INDEX clubs_unreminded_events_idx;

ALTER TABLE club_members
    DROP COLUMN maybe;

ALTER TABLE clubs
    DROP CONSTRAINT clubs_closed_check,
    DROP CONSTRAINT clubs_schedule_check,
    DROP COLUMN reminded_at,
    DROP COLUMN maybe_count,
    DROP COLUMN ends_at,
    DROP COLUMN starts_at,
    DROP COLUMN kind;

-- +goose Down
-- Every club is a group again; the deleted events do not come back.
ALTER TABLE clubs
    ADD COLUMN kind        text        NOT NULL DEFAULT 'group' CHECK (kind IN ('group', 'event')),
    ADD COLUMN starts_at   timestamptz,
    ADD COLUMN ends_at     timestamptz,
    ADD COLUMN maybe_count integer     NOT NULL DEFAULT 0 CHECK (maybe_count >= 0),
    ADD COLUMN reminded_at timestamptz,
    ADD CONSTRAINT clubs_schedule_check CHECK (
        CASE kind
            WHEN 'event' THEN starts_at IS NOT NULL AND (ends_at IS NULL OR ends_at > starts_at)
            ELSE starts_at IS NULL AND ends_at IS NULL
        END
    ),
    ADD CONSTRAINT clubs_closed_check CHECK (NOT closed OR kind = 'group');

ALTER TABLE club_members
    ADD COLUMN maybe boolean NOT NULL DEFAULT false;

CREATE INDEX clubs_unreminded_events_idx ON clubs (starts_at) WHERE kind = 'event' AND reminded_at IS NULL;
