-- +goose Up
-- post keeps a copy of who is in which club, for the news, and hears of
-- everyone who joins and leaves through the outbox from now on. This tells
-- it the members there already are, the oldest first; a member told twice
-- is one member. The payload is the club's ID and then the member's.
INSERT INTO outbox (stream, kind, payload)
SELECT 'post', 'join_club', uuid_send(club_id) || uuid_send(user_id)
FROM club_members
ORDER BY id;

-- +goose Down
DELETE FROM outbox WHERE stream = 'post';
