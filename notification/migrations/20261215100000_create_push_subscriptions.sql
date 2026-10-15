-- +goose Up
-- The browsers that take the pushes of a user (Web Push): where to push
-- (endpoint, the secret address the browser's push service gave) and the
-- keys to encrypt with. One browser pushes to one user, whoever
-- subscribed it last.
CREATE TABLE push_subscriptions (
    id         uuid        PRIMARY KEY,
    user_id    uuid        NOT NULL,
    endpoint   text        NOT NULL UNIQUE CHECK (length(endpoint) BETWEEN 1 AND 2048),
    p256dh     bytea       NOT NULL CHECK (length(p256dh) = 65),
    auth       bytea       NOT NULL CHECK (length(auth) = 16),
    created_at timestamptz NOT NULL
);

CREATE INDEX push_subscriptions_user_idx ON push_subscriptions (user_id, id);

-- +goose Down
DROP TABLE push_subscriptions;
