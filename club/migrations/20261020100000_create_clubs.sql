-- +goose Up
-- A community. address is the short name of its page, NULL for none:
-- unique whatever the case.
CREATE TABLE clubs (
    id           uuid        PRIMARY KEY,
    name         text        NOT NULL CHECK (name <> '' AND char_length(name) <= 64),
    description  text        NOT NULL DEFAULT '' CHECK (char_length(description) <= 1000),
    address      text,
    wall         text        NOT NULL DEFAULT 'open' CHECK (wall IN ('open', 'closed')),
    -- Changed by the statement that adds or removes a member.
    member_count integer     NOT NULL DEFAULT 0 CHECK (member_count >= 0),
    created_at   timestamptz NOT NULL
);

CREATE UNIQUE INDEX clubs_address_key ON clubs (lower(address));

-- Who is in a club. user_id refers to the user service's database, so
-- there is no foreign key. id is a UUIDv7: the order of joining and the
-- cursor of the lists.
CREATE TABLE club_members (
    id      uuid PRIMARY KEY,
    club_id uuid NOT NULL REFERENCES clubs (id) ON DELETE CASCADE,
    user_id uuid NOT NULL,
    role    text NOT NULL CHECK (role IN ('member', 'admin', 'owner')),
    UNIQUE (club_id, user_id)
);

-- One owner a club at most.
CREATE UNIQUE INDEX club_members_owner_key ON club_members (club_id) WHERE role = 'owner';

-- The members of a club and the clubs of a user, the newest first.
CREATE INDEX club_members_club_idx ON club_members (club_id, id);
CREATE INDEX club_members_user_idx ON club_members (user_id, id);

-- +goose Down
DROP TABLE club_members;
DROP TABLE clubs;
