-- +goose Up
-- post keeps a copy of whom everyone reads, for the news, and hears of
-- each follow through the outbox from now on. This tells it the follows
-- there already are, the oldest first; a follow told twice is one follow.
-- The payload is the follower's ID and then the followee's.
INSERT INTO outbox (stream, kind, payload)
SELECT 'post', 'follow_source', uuid_send(follower_id) || uuid_send(followee_id)
FROM follows
ORDER BY id;

-- +goose Down
DELETE FROM outbox WHERE stream = 'post' AND kind IN ('follow_source', 'unfollow_source');
