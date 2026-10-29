-- +goose Up
-- "Знакомства": the card a person shows and who liked or skipped whom.
CREATE TABLE dating_cards (
    user_id    uuid        PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    goal       text        NOT NULL CHECK (goal IN ('friendship', 'chat', 'love')),
    seek       text        NOT NULL CHECK (seek IN ('', 'male', 'female')),
    about      text        NOT NULL CHECK (char_length(about) <= 300),
    is_on      boolean     NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE INDEX dating_cards_on_idx ON dating_cards (updated_at DESC, user_id DESC) WHERE is_on;

CREATE TABLE dating_answers (
    from_id     uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    to_id       uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    is_like     boolean     NOT NULL,
    answered_at timestamptz NOT NULL,
    PRIMARY KEY (from_id, to_id),
    CHECK (from_id <> to_id)
);

CREATE INDEX dating_answers_to_idx ON dating_answers (to_id, from_id) WHERE is_like;

-- +goose Down
DROP TABLE dating_answers;
DROP TABLE dating_cards;
