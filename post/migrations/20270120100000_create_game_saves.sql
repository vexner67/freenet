-- +goose Up
-- What HTML widgets remember of their players: one save of each player in
-- each game (what freenet.save of the sandbox hands over) and the best
-- score (freenet.score). A deleted widget takes them with it; user_id
-- refers to the user service's database, so there is no foreign key.
CREATE TABLE game_saves (
    game_id    uuid        NOT NULL REFERENCES documents (id) ON DELETE CASCADE,
    user_id    uuid        NOT NULL,
    data       text        NOT NULL CHECK (octet_length(data) <= 65536),
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (game_id, user_id)
);

CREATE TABLE game_scores (
    game_id    uuid        NOT NULL REFERENCES documents (id) ON DELETE CASCADE,
    user_id    uuid        NOT NULL,
    score      bigint      NOT NULL CHECK (score BETWEEN 0 AND 9007199254740991),
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (game_id, user_id)
);

-- The deletion of a page forgets its saves and scores.
CREATE INDEX game_saves_user_idx ON game_saves (user_id);
CREATE INDEX game_scores_user_idx ON game_scores (user_id);

-- +goose Down
DROP TABLE game_scores;
DROP TABLE game_saves;
