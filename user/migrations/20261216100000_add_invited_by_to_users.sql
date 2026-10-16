-- +goose Up
-- Who invited the page ("Пригласить" of openvk): named once, by the page
-- itself, soon after signing up. An erased inviter leaves nobody named.
ALTER TABLE users
    ADD COLUMN invited_by uuid REFERENCES users (id) ON DELETE SET NULL,
    ADD CONSTRAINT users_invited_by_check CHECK (invited_by <> id);

-- For the foreign key: erasing a page finds whom it invited.
CREATE INDEX users_invited_by_idx ON users (invited_by) WHERE invited_by IS NOT NULL;

-- +goose Down
DROP INDEX users_invited_by_idx;

ALTER TABLE users
    DROP COLUMN invited_by;
