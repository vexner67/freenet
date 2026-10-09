-- +goose Up
-- A post shared into a dialog (openvk's "Поделиться" "в переписку"): its
-- ID in the post service, which keeps it; a deleted one shows as such. A
-- message says something: text, photos or a post.
ALTER TABLE messages
    ADD COLUMN post_id uuid,
    DROP CONSTRAINT messages_content_check,
    ADD CONSTRAINT messages_content_check CHECK (text <> '' OR cardinality(photo_ids) > 0 OR post_id IS NOT NULL);

-- +goose Down
DELETE FROM messages WHERE text = '' AND cardinality(photo_ids) = 0;
ALTER TABLE messages
    DROP CONSTRAINT messages_content_check,
    ADD CONSTRAINT messages_content_check CHECK (text <> '' OR cardinality(photo_ids) > 0),
    DROP COLUMN post_id;
