-- +goose Up
-- How the site is dressed for the New Year: one row, which the
-- administrators change. auto — by the calendar, from December 15 to
-- January 14.
CREATE TABLE site_decor (
    only_row boolean PRIMARY KEY DEFAULT true CHECK (only_row),
    mode text NOT NULL CHECK (mode IN ('auto', 'on', 'off'))
);

INSERT INTO site_decor (mode) VALUES ('auto');

-- +goose Down
DROP TABLE site_decor;
