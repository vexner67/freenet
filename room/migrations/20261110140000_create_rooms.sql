-- +goose Up
CREATE TABLE rooms (
 id uuid PRIMARY KEY,
 owner_id uuid NOT NULL,
 club_id uuid,
 title text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 64 AND title !~ '[[:cntrl:]]'),
 kind text NOT NULL CHECK (kind IN ('talk','watch','listen')),
 access text NOT NULL CHECK (access IN ('everyone','friends','link','club')),
 control text NOT NULL CHECK (control IN ('owner','everyone')),
 permanent boolean NOT NULL,
 created_at timestamptz NOT NULL,
 closed_at timestamptz,
 CHECK (access <> 'club' OR club_id IS NOT NULL)
);
CREATE INDEX rooms_owner_open ON rooms (owner_id, id DESC) WHERE closed_at IS NULL;
CREATE INDEX rooms_club_open ON rooms (club_id, id DESC) WHERE closed_at IS NULL;
CREATE INDEX rooms_open ON rooms (id DESC) WHERE closed_at IS NULL;
CREATE TABLE room_bans (
 room_id uuid NOT NULL REFERENCES rooms (id) ON DELETE CASCADE,
 user_id uuid NOT NULL,
 until timestamptz NOT NULL,
 PRIMARY KEY (room_id,user_id)
);

-- +goose Down
DROP TABLE room_bans;
DROP TABLE rooms;
