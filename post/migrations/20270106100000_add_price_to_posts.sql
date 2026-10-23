-- +goose Up
-- What others pay to see the photos and documents of a post; 0 — they are
-- free. Only a post on its author's own wall, with photos or documents,
-- asks it (post.Priced).
ALTER TABLE posts
    ADD COLUMN price bigint NOT NULL DEFAULT 0 CHECK (price BETWEEN 0 AND 1000000);

-- +goose Down
ALTER TABLE posts
    DROP COLUMN price;
