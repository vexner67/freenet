-- +goose Up
-- Hearts of topic messages, as of comments: one per user, counted in
-- like_count by the same statement, deleted with the message.
ALTER TABLE topic_messages
    ADD COLUMN like_count integer NOT NULL DEFAULT 0 CHECK (like_count >= 0);

CREATE TABLE topic_message_likes (
    message_id uuid        NOT NULL REFERENCES topic_messages (id) ON DELETE CASCADE,
    user_id    uuid        NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (message_id, user_id)
);

-- Erasing a user finds their likes by it.
CREATE INDEX topic_message_likes_user_id_idx ON topic_message_likes (user_id);

-- +goose Down
DROP TABLE topic_message_likes;

ALTER TABLE topic_messages
    DROP COLUMN like_count;
