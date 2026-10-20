-- +goose Up
-- The ads of the global feed, which the administrators make. The cover
-- is a photo in media, kept under the administrator who uploaded it.
CREATE TABLE ads (
    id uuid PRIMARY KEY,
    url text NOT NULL CHECK (char_length(url) BETWEEN 1 AND 2048),
    title text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 100),
    body text NOT NULL DEFAULT '' CHECK (char_length(body) <= 500),
    photo_owner_id uuid,
    photo_id uuid,
    active boolean NOT NULL DEFAULT true,
    CHECK ((photo_id IS NULL) = (photo_owner_id IS NULL))
);

-- +goose Down
DROP TABLE ads;
