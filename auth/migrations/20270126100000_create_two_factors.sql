-- +goose Up
-- The second step of signing in: the secret of a page's authenticator app,
-- sealed with a key of auth (AES-GCM), the step of the last code accepted,
-- so that a code works once, and the hashes of the recovery codes left.
-- A factor set up and not yet enabled waits for its first code.
CREATE TABLE two_factors (
    user_id         uuid PRIMARY KEY,
    sealed_secret   bytea NOT NULL,
    enabled         boolean NOT NULL DEFAULT false,
    last_step       bigint NOT NULL DEFAULT 0 CHECK (last_step >= 0),
    recovery_hashes text[] NOT NULL DEFAULT '{}',
    updated_at      timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE two_factors;
