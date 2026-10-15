-- +goose Up
-- reply_to_id: the message of the same dialog or chat a message answers
-- (no foreign key: the answer outlives what it answered and shows it
-- gone). The reactions: one of each person to a message, gone with it.
ALTER TABLE messages
    ADD COLUMN reply_to_id uuid;

ALTER TABLE chat_messages
    ADD COLUMN reply_to_id uuid;

CREATE TABLE message_reactions (
    message_id uuid NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
    user_id uuid NOT NULL,
    reaction text NOT NULL CHECK (reaction IN ('heart', 'like', 'laugh', 'wow', 'sad', 'fire')),
    created_at timestamptz NOT NULL,
    PRIMARY KEY (message_id, user_id)
);

CREATE TABLE chat_message_reactions (
    message_id uuid NOT NULL REFERENCES chat_messages (id) ON DELETE CASCADE,
    user_id uuid NOT NULL,
    reaction text NOT NULL CHECK (reaction IN ('heart', 'like', 'laugh', 'wow', 'sad', 'fire')),
    created_at timestamptz NOT NULL,
    PRIMARY KEY (message_id, user_id)
);

-- Finds the reactions of an erased page, which stays in other chats'
-- messages; those of dialogs go with the dialogs.
CREATE INDEX chat_message_reactions_user_id_idx ON chat_message_reactions (user_id);

-- +goose Down
DROP TABLE chat_message_reactions;

DROP TABLE message_reactions;

ALTER TABLE chat_messages
    DROP COLUMN reply_to_id;

ALTER TABLE messages
    DROP COLUMN reply_to_id;
