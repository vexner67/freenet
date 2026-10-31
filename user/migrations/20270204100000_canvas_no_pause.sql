-- +goose Up
-- The canvas has no pause any more, only a rate limit, and a palette of 64.
ALTER TABLE canvas_cells DROP CONSTRAINT canvas_cells_color_check;
ALTER TABLE canvas_cells ADD CONSTRAINT canvas_cells_color_check CHECK (color >= 0 AND color < 64);
DROP TABLE canvas_painters;

-- +goose Down
CREATE TABLE canvas_painters (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    painted_at timestamptz NOT NULL
);
UPDATE canvas_cells SET color = 0 WHERE color >= 16;
ALTER TABLE canvas_cells DROP CONSTRAINT canvas_cells_color_check;
ALTER TABLE canvas_cells ADD CONSTRAINT canvas_cells_color_check CHECK (color >= 0 AND color < 16);
