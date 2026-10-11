-- +goose Up
-- shared_photo_id: a photo of an album of the post service shared into the
-- dialog ("Поделиться" under a photo, "в переписку"). No foreign key, as
-- with post_id: a deleted photo leaves the message.
ALTER TABLE messages
    ADD COLUMN shared_photo_id uuid,
    DROP CONSTRAINT messages_content_check,
    ADD CONSTRAINT messages_content_check CHECK (
        deleted_at IS NOT NULL
        OR text <> ''
        OR cardinality(photo_ids) > 0
        OR post_id IS NOT NULL
        OR shared_photo_id IS NOT NULL
        OR jsonb_array_length(forwarded) > 0
    );

-- +goose Down
-- A message that only shared a photo says so in words: it may not be empty.
UPDATE messages
SET text = 'Фотография'
WHERE shared_photo_id IS NOT NULL
    AND text = '';

ALTER TABLE messages
    DROP CONSTRAINT messages_content_check,
    ADD CONSTRAINT messages_content_check CHECK (
        deleted_at IS NOT NULL
        OR text <> ''
        OR cardinality(photo_ids) > 0
        OR post_id IS NOT NULL
        OR jsonb_array_length(forwarded) > 0
    ),
    DROP COLUMN shared_photo_id;
