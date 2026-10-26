-- +goose Up
-- Titles (user.Title): awards with a name and a picture the administrators
-- give a page, as many times as they like; the profile shows them, the
-- newest first.
CREATE TABLE titles (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 40),
    icon text NOT NULL CHECK (icon IN ('crown', 'cup', 'star', 'laurel', 'shield', 'flame', 'diamond', 'lightning')),
    granted_at timestamptz NOT NULL
);

CREATE INDEX titles_user_idx ON titles (user_id, id DESC);

-- +goose Down
DROP TABLE titles;
