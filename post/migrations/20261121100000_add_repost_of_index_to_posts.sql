-- +goose Up
-- Who shared a post ("Поделились N человек"), newest first, and whose
-- notifications a deleted post takes back.
CREATE INDEX posts_repost_of_idx ON posts (repost_of, id DESC) WHERE repost_of IS NOT NULL;

-- +goose Down
DROP INDEX posts_repost_of_idx;
