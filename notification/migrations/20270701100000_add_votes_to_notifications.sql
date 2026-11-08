-- +goose Up
-- A transfer of votes tells how many and its message, as openvk's
-- "передал вам 10 голосов. Сообщение: …"; 0 is one told before.
ALTER TABLE notifications
    ADD COLUMN votes bigint NOT NULL DEFAULT 0,
    ADD COLUMN note text NOT NULL DEFAULT '';

ALTER TABLE notifications
    ADD CONSTRAINT notifications_votes_check CHECK (
        votes >= 0
        AND char_length(note) <= 140
        AND (kind = 'votes' OR (votes = 0 AND note = ''))
    );

-- +goose Down
ALTER TABLE notifications
    DROP CONSTRAINT notifications_votes_check;

ALTER TABLE notifications
    DROP COLUMN note,
    DROP COLUMN votes;
