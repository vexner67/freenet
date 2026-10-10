-- +goose Up
-- An event ("встреча" of openvk) is a club of its kind with the time it
-- starts and, maybe, ends. Its members come for sure or, with maybe,
-- only might ("Возможно придут"); maybe_count counts those, changed by
-- the statement that adds or removes a member.
ALTER TABLE clubs
    ADD COLUMN kind        text        NOT NULL DEFAULT 'group' CHECK (kind IN ('group', 'event')),
    ADD COLUMN starts_at   timestamptz,
    ADD COLUMN ends_at     timestamptz,
    ADD COLUMN maybe_count integer     NOT NULL DEFAULT 0 CHECK (maybe_count >= 0),
    ADD CONSTRAINT clubs_schedule_check CHECK (
        CASE kind
            WHEN 'event' THEN starts_at IS NOT NULL AND (ends_at IS NULL OR ends_at > starts_at)
            ELSE starts_at IS NULL AND ends_at IS NULL
        END
    );

ALTER TABLE club_members
    ADD COLUMN maybe boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE club_members
    DROP COLUMN maybe;

-- Events go: a group has no schedule to be told apart by.
DELETE FROM clubs WHERE kind = 'event';

ALTER TABLE clubs
    DROP CONSTRAINT clubs_schedule_check,
    DROP COLUMN maybe_count,
    DROP COLUMN ends_at,
    DROP COLUMN starts_at,
    DROP COLUMN kind;
