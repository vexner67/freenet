-- +goose Up
-- The pages opened to the Fediverse; a page closed again keeps its keys
-- (open = false): what it sent is signed with them.
CREATE TABLE accounts (
    user_id     uuid        PRIMARY KEY,
    public_key  text        NOT NULL CHECK (public_key <> ''),
    private_key text        NOT NULL CHECK (private_key <> ''),
    open        boolean     NOT NULL,
    created_at  timestamptz NOT NULL
);

-- Who of other servers follows whom here, and where their server takes
-- the posts.
CREATE TABLE followers (
    user_id    uuid        NOT NULL REFERENCES accounts (user_id) ON DELETE CASCADE,
    actor      text        NOT NULL CHECK (actor LIKE 'https://%'),
    inbox      text        NOT NULL CHECK (inbox LIKE 'https://%'),
    created_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, actor)
);

CREATE INDEX followers_actor_idx ON followers (actor);

-- The documents of remote actors read lately: their inboxes and keys.
CREATE TABLE remote_actors (
    id           text        PRIMARY KEY CHECK (id LIKE 'https://%'),
    inbox        text        NOT NULL,
    shared_inbox text        NOT NULL,
    key_id       text        NOT NULL,
    public_key   text        NOT NULL,
    fetched_at   timestamptz NOT NULL
);

-- The activities to send to other servers, recorded in the transaction
-- of what caused them and delivered by a relay (pkg/outbox).
CREATE TABLE outbox (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    stream     text        NOT NULL,
    kind       text        NOT NULL,
    payload    bytea       NOT NULL,
    request_id text        NOT NULL DEFAULT '',
    attempts   integer     NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    due_at     timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX outbox_stream_idx ON outbox (stream, id);

-- +goose Down
DROP TABLE outbox;
DROP TABLE remote_actors;
DROP TABLE followers;
DROP TABLE accounts;
