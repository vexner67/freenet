-- +goose Up
-- The walls not everyone reads: of a person whose profile is closed
-- ("Тип профиля: закрытый" of openvk) and of a closed community. The
-- user and club services tell which; no foreign key, they live there.
CREATE TABLE closed_walls (
    wall_id uuid PRIMARY KEY,
    club    boolean NOT NULL
);

-- Whether reader (NULL: nobody signed in) is kept from a wall. A closed
-- page is read by its owner and by those the owner follows, a closed
-- community by its members: both are what feed_sources already knows —
-- whom the owner reads, and who reads the community.
-- +goose StatementBegin
CREATE FUNCTION wall_closed_to(wall uuid, reader uuid) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT EXISTS (
        SELECT 1
        FROM closed_walls c
        WHERE c.wall_id = wall
            AND wall IS DISTINCT FROM reader
            AND NOT EXISTS (
                SELECT 1
                FROM feed_sources fs
                WHERE (c.club AND fs.reader_id = reader AND fs.source_id = wall)
                    OR (NOT c.club AND fs.reader_id = wall AND fs.source_id = reader AND NOT fs.club)
            )
    )
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION wall_closed_to(uuid, uuid);
DROP TABLE closed_walls;
