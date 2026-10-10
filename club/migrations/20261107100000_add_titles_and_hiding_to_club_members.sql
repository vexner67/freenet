-- +goose Up
-- What an admin does in the club, shown under their name (openvk's comment
-- on an admin), and whether the club hides that they run it: only those
-- who run a club have either.
ALTER TABLE club_members
    ADD COLUMN title  text    NOT NULL DEFAULT '' CHECK (char_length(title) <= 36),
    ADD COLUMN hidden boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT club_members_seat_check CHECK (role <> 'member' OR (title = '' AND NOT hidden));

-- +goose Down
ALTER TABLE club_members
    DROP CONSTRAINT club_members_seat_check,
    DROP COLUMN hidden,
    DROP COLUMN title;
