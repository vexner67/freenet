-- +goose Up
-- post now leaves what a deleted page wrote out of its lists and counts,
-- and hears of a deleted page as it hears of its reactions to hold. This
-- tells it the pages that are deleted already; a page told twice is held
-- once. The payload is the ID of the page.
INSERT INTO outbox (stream, kind, payload)
SELECT 'post', 'hold_reactions', uuid_send(id)
FROM users
WHERE deactivated_at IS NOT NULL
ORDER BY deactivated_at;

-- +goose Down
-- Nothing to take back: what was delivered holds until the page is
-- restored or erased, as any deleted page.
