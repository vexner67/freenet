-- +goose Up
-- A closed group ("Закрытая" of openvk's "Тип группы"): one joins it by a
-- request its admins accept, and only its members see what is in it.
ALTER TABLE clubs
    ADD COLUMN closed boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT clubs_closed_check CHECK (NOT closed OR kind = 'group');

-- Who asked to join a closed group and waits for its admins ("Заявки").
CREATE TABLE club_requests (
    id      uuid PRIMARY KEY,
    club_id uuid NOT NULL REFERENCES clubs (id) ON DELETE CASCADE,
    user_id uuid NOT NULL,
    UNIQUE (club_id, user_id)
);

-- The requests of a club, the newest first, and those of an erased user.
CREATE INDEX club_requests_club_idx ON club_requests (club_id, id);
CREATE INDEX club_requests_user_idx ON club_requests (user_id);

-- +goose Down
-- post was told of the closed groups: tell it they are open again.
INSERT INTO outbox (stream, kind, payload)
SELECT 'post', 'open_wall', uuid_send(id)
FROM clubs
WHERE closed;

DROP TABLE club_requests;

ALTER TABLE clubs
    DROP CONSTRAINT clubs_closed_check,
    DROP COLUMN closed;
