-- +goose Up
CREATE TABLE bank_cards (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    number text NOT NULL UNIQUE CHECK (number ~ '^1919[0-9]{12}$'),
    cover text NOT NULL,
    issued_at timestamptz NOT NULL
);

-- +goose Down
DROP TABLE bank_cards;
