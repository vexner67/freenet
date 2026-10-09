-- +goose Up
-- A post and a comment are as long as openvk lets them be, 60000
-- characters; suggestions and the messages of topics checked the old 1000.
ALTER TABLE suggestions
    DROP CONSTRAINT suggestions_text_check,
    ADD CONSTRAINT suggestions_text_check CHECK (char_length(text) <= 60000);
ALTER TABLE topic_messages
    DROP CONSTRAINT topic_messages_text_check,
    ADD CONSTRAINT topic_messages_text_check CHECK (char_length(text) <= 60000);

-- +goose Down
-- NOT VALID: longer texts written meanwhile stay.
ALTER TABLE topic_messages
    DROP CONSTRAINT topic_messages_text_check,
    ADD CONSTRAINT topic_messages_text_check CHECK (char_length(text) <= 1000) NOT VALID;
ALTER TABLE suggestions
    DROP CONSTRAINT suggestions_text_check,
    ADD CONSTRAINT suggestions_text_check CHECK (char_length(text) <= 1000) NOT VALID;
