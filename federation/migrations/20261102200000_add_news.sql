-- +goose Up
-- How remote actors show themselves, for the news.
ALTER TABLE remote_actors
    ADD COLUMN name     text NOT NULL DEFAULT '',
    ADD COLUMN username text NOT NULL DEFAULT '',
    ADD COLUMN icon     text NOT NULL DEFAULT '',
    ADD COLUMN page     text NOT NULL DEFAULT '';

-- The accounts of other servers the site follows: featured ones for the
-- news of everyone, the others for those who subscribed to them.
CREATE TABLE following (
    actor      text        PRIMARY KEY CHECK (actor LIKE 'https://%'),
    follow_id  uuid        NOT NULL UNIQUE,
    accepted   boolean     NOT NULL,
    featured   boolean     NOT NULL,
    created_at timestamptz NOT NULL
);

-- Whom of other servers the people here read.
CREATE TABLE subscriptions (
    user_id    uuid        NOT NULL,
    actor      text        NOT NULL CHECK (actor LIKE 'https://%'),
    created_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, actor)
);

CREATE INDEX subscriptions_actor_idx ON subscriptions (actor);

-- The notes of the accounts the site follows, for the news; their IDs
-- are UUIDv7 of when they were published.
CREATE TABLE remote_posts (
    id        uuid        PRIMARY KEY,
    uri       text        NOT NULL UNIQUE CHECK (uri LIKE 'https://%'),
    actor     text        NOT NULL,
    page      text        NOT NULL,
    text      text        NOT NULL,
    summary   text        NOT NULL,
    images    jsonb       NOT NULL,
    sensitive boolean     NOT NULL,
    language  text        NOT NULL,
    published timestamptz NOT NULL,
    updated   timestamptz
);

CREATE INDEX remote_posts_actor_idx ON remote_posts (actor, id);
CREATE INDEX remote_posts_published_idx ON remote_posts (published);

-- +goose Down
DROP TABLE remote_posts;
DROP TABLE subscriptions;
DROP TABLE following;

ALTER TABLE remote_actors
    DROP COLUMN page,
    DROP COLUMN icon,
    DROP COLUMN username,
    DROP COLUMN name;
