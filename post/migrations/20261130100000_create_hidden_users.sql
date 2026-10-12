-- +goose Up
-- The pages their owners deleted and may still restore. Nobody sees what
-- such a page wrote, so the lists leave it out and the counts do not
-- count it: a list is never shorter than its counter says. The rows come
-- and go with the reactions of the page (held_reactions); no foreign key,
-- users live in another service.
CREATE TABLE hidden_users (
    user_id uuid PRIMARY KEY
);

-- The archived reposts of a post, which "who shared it" does not show.
CREATE INDEX posts_archived_reposts_idx ON posts (repost_of) WHERE archived AND repost_of IS NOT NULL;

-- Whether nobody sees a post: the owner of its wall deleted the page, or
-- its author did and the post does not speak for the wall.
-- +goose StatementBegin
CREATE FUNCTION post_hidden(wall uuid, author uuid, for_wall boolean) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT EXISTS (
        SELECT 1
        FROM hidden_users h
        WHERE h.user_id = wall
            OR (h.user_id = author AND NOT for_wall)
    )
$$;
-- +goose StatementEnd

-- Whether nobody sees what a person wrote in their own name.
-- +goose StatementBegin
CREATE FUNCTION user_hidden(person uuid) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT EXISTS (
        SELECT 1
        FROM hidden_users h
        WHERE h.user_id = person
    )
$$;
-- +goose StatementEnd

-- The functions below tell how many of the things a counter counts nobody
-- sees. Each starts from hidden_users, which is small and mostly empty,
-- and reads the index by author: a counter costs nothing to correct while
-- no page is deleted.

-- +goose StatementBegin
CREATE FUNCTION hidden_comments(post uuid) RETURNS integer
LANGUAGE sql STABLE AS $$
    SELECT count(*)::integer
    FROM hidden_users h
    JOIN comments c ON c.author_id = h.user_id
    WHERE c.post_id = post
        AND NOT c.from_wall
$$;
-- +goose StatementEnd

-- The reposts "who shared it" leaves out: the archived ones and those
-- nobody sees.
-- +goose StatementBegin
CREATE FUNCTION unseen_reposts(post uuid) RETURNS integer
LANGUAGE sql STABLE AS $$
    SELECT (
        SELECT count(*)
        FROM posts r
        WHERE r.repost_of = post
            AND r.archived
    )::integer + (
        SELECT count(*)
        FROM (
            SELECT r.id
            FROM hidden_users h
            JOIN posts r ON r.author_id = h.user_id
            WHERE r.repost_of = post
                AND NOT r.archived
                AND NOT r.from_wall
            UNION
            SELECT r.id
            FROM hidden_users h
            JOIN posts r ON r.wall_id = h.user_id
            WHERE r.repost_of = post
                AND NOT r.archived
        ) hidden
    )::integer
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION hidden_topic_messages(topic uuid) RETURNS integer
LANGUAGE sql STABLE AS $$
    SELECT count(*)::integer
    FROM hidden_users h
    JOIN topic_messages m ON m.author_id = h.user_id
    WHERE m.topic_id = topic
        AND NOT m.from_wall
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION hidden_album_photos(album uuid) RETURNS integer
LANGUAGE sql STABLE AS $$
    SELECT count(*)::integer
    FROM hidden_users h
    JOIN album_photos p ON p.author_id = h.user_id
    WHERE p.album_id = album
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION hidden_photo_comments(photo uuid) RETURNS integer
LANGUAGE sql STABLE AS $$
    SELECT count(*)::integer
    FROM hidden_users h
    JOIN photo_comments c ON c.author_id = h.user_id
    WHERE c.photo_id = photo
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION hidden_note_comments(note uuid) RETURNS integer
LANGUAGE sql STABLE AS $$
    SELECT count(*)::integer
    FROM hidden_users h
    JOIN note_comments c ON c.author_id = h.user_id
    WHERE c.note_id = note
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION hidden_note_comments(uuid);
DROP FUNCTION hidden_photo_comments(uuid);
DROP FUNCTION hidden_album_photos(uuid);
DROP FUNCTION hidden_topic_messages(uuid);
DROP FUNCTION unseen_reposts(uuid);
DROP FUNCTION hidden_comments(uuid);
DROP FUNCTION user_hidden(uuid);
DROP FUNCTION post_hidden(uuid, uuid, boolean);
DROP INDEX posts_archived_reposts_idx;
-- The pages deleted now are counted again until they are deleted anew.
DROP TABLE hidden_users;
