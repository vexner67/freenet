-- +goose Up
-- A visit used to rewrite the row of the person in users, a wide row
-- under trigram indexes, every two minutes they stayed on the site. The
-- visits get a table of their own: two columns and no index but the key,
-- so a new visit overwrites the old one in place (the pages keep room for
-- that), and many visits are written by one statement.
--
-- users.last_seen_at stays, unread, for the version before this one to
-- run on this schema if it is rolled back to; a later migration drops it.
CREATE TABLE presence (
    user_id      uuid        PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    last_seen_at timestamptz NOT NULL
) WITH (fillfactor = 70);

INSERT INTO presence (
    user_id,
    last_seen_at
)
SELECT
    id,
    last_seen_at
FROM users
WHERE last_seen_at IS NOT NULL;

-- +goose Down
UPDATE users u
SET last_seen_at = p.last_seen_at
FROM presence p
WHERE p.user_id = u.id
    AND (u.last_seen_at IS NULL OR u.last_seen_at < p.last_seen_at);

DROP TABLE presence;
