-- +goose Up
-- user_id refers to the user service's database, so there is no foreign key.
CREATE TABLE sessions (
    id                 uuid        PRIMARY KEY,
    user_id            uuid        NOT NULL,
    refresh_token_hash text        NOT NULL,
    created_at         timestamptz NOT NULL
);

-- +goose Down
DROP TABLE sessions;
