-- +goose Up
-- The shared canvas: the cells ever painted, each with who painted it last
-- (NULL once their page is erased) and when, and when each person last
-- painted, which holds them back a minute.
CREATE TABLE canvas_cells (
    x smallint NOT NULL CHECK (x >= 0 AND x < 100),
    y smallint NOT NULL CHECK (y >= 0 AND y < 100),
    color smallint NOT NULL CHECK (color >= 0 AND color < 16),
    user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    painted_at timestamptz NOT NULL,
    PRIMARY KEY (x, y)
);

CREATE TABLE canvas_painters (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    painted_at timestamptz NOT NULL
);

-- +goose Down
DROP TABLE canvas_painters;
DROP TABLE canvas_cells;
