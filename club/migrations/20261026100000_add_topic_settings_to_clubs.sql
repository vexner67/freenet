-- +goose Up
-- "Обсуждения" of openvk's settings: anyone starts topics, not only the
-- admins ("Все могут создавать темы"), and the page lists the topics above
-- the wall rather than in the right column.
ALTER TABLE clubs
    ADD COLUMN open_topics boolean NOT NULL DEFAULT false,
    ADD COLUMN topics_above_wall boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE clubs
    DROP COLUMN topics_above_wall,
    DROP COLUMN open_topics;
