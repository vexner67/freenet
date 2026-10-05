-- +goose Up
CREATE TABLE users (
    id              uuid        PRIMARY KEY,
    email           text        NOT NULL,
    name            text        NOT NULL DEFAULT '',
    surname         text        NOT NULL DEFAULT '',
    nickname        text        NOT NULL DEFAULT '',
    status          text        NOT NULL DEFAULT '',
    gender          text        NOT NULL DEFAULT '',
    relationship    text        NOT NULL DEFAULT '',
    worldview       text        NOT NULL DEFAULT '',
    political_views text        NOT NULL DEFAULT '',
    birthday        date,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

-- Emails and nicknames are unique regardless of case. A user who has not
-- filled in the profile yet has an empty nickname, which is not unique.
CREATE UNIQUE INDEX users_email_uidx ON users (lower(email));
CREATE UNIQUE INDEX users_nickname_uidx ON users (lower(nickname)) WHERE nickname <> '';

-- +goose Down
DROP TABLE users;
