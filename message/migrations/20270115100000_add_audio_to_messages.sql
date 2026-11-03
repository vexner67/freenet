-- +goose Up
-- audio_ids: songs of "Аудиозаписи" the message attaches, which the post
-- service keeps (no foreign key: a deleted song leaves the message). As
-- documents, they are content enough, but not of an action and not with a
-- sticker; those of a forwarded message go in its JSON.
ALTER TABLE messages
    ADD COLUMN audio_ids uuid[] NOT NULL DEFAULT '{}' CHECK (cardinality(audio_ids) <= 10),
    DROP CONSTRAINT messages_content_check,
    ADD CONSTRAINT messages_content_check CHECK (
        deleted_at IS NOT NULL
        OR sealed IS NOT NULL
        OR cardinality(photo_ids) > 0
        OR post_id IS NOT NULL
        OR shared_photo_id IS NOT NULL
        OR jsonb_array_length(forwarded) > 0
        OR sticker <> ''
        OR cardinality(document_ids) > 0
        OR cardinality(audio_ids) > 0
    ),
    ADD CONSTRAINT messages_audio_check CHECK (sticker = '' OR cardinality(audio_ids) = 0);

ALTER TABLE chat_messages
    ADD COLUMN audio_ids uuid[] NOT NULL DEFAULT '{}' CHECK (cardinality(audio_ids) <= 10),
    DROP CONSTRAINT chat_messages_content_check,
    ADD CONSTRAINT chat_messages_content_check CHECK (
        action <> ''
        OR deleted_at IS NOT NULL
        OR text <> ''
        OR cardinality(photo_ids) > 0
        OR post_id IS NOT NULL
        OR shared_photo_id IS NOT NULL
        OR forwarded <> '[]'
        OR sticker <> ''
        OR cardinality(document_ids) > 0
        OR cardinality(audio_ids) > 0
    ),
    ADD CONSTRAINT chat_messages_audio_check CHECK ((action = '' AND sticker = '') OR cardinality(audio_ids) = 0);

-- +goose Down
-- A message of songs alone would say nothing: it is taken back, as its
-- sender could have.
UPDATE chat_messages
SET deleted_at = now()
WHERE deleted_at IS NULL AND action = '' AND text = '' AND cardinality(photo_ids) = 0 AND post_id IS NULL
    AND shared_photo_id IS NULL AND forwarded = '[]' AND sticker = '' AND cardinality(document_ids) = 0
    AND cardinality(audio_ids) > 0;

UPDATE messages
SET deleted_at = now()
WHERE deleted_at IS NULL AND sealed IS NULL AND cardinality(photo_ids) = 0 AND post_id IS NULL
    AND shared_photo_id IS NULL AND jsonb_array_length(forwarded) = 0 AND sticker = '' AND cardinality(document_ids) = 0
    AND cardinality(audio_ids) > 0;

ALTER TABLE chat_messages
    DROP CONSTRAINT chat_messages_audio_check,
    DROP CONSTRAINT chat_messages_content_check,
    ADD CONSTRAINT chat_messages_content_check CHECK (
        action <> ''
        OR deleted_at IS NOT NULL
        OR text <> ''
        OR cardinality(photo_ids) > 0
        OR post_id IS NOT NULL
        OR shared_photo_id IS NOT NULL
        OR forwarded <> '[]'
        OR sticker <> ''
        OR cardinality(document_ids) > 0
    ),
    DROP COLUMN audio_ids;

ALTER TABLE messages
    DROP CONSTRAINT messages_audio_check,
    DROP CONSTRAINT messages_content_check,
    ADD CONSTRAINT messages_content_check CHECK (
        deleted_at IS NOT NULL
        OR sealed IS NOT NULL
        OR cardinality(photo_ids) > 0
        OR post_id IS NOT NULL
        OR shared_photo_id IS NOT NULL
        OR jsonb_array_length(forwarded) > 0
        OR sticker <> ''
        OR cardinality(document_ids) > 0
    ),
    DROP COLUMN audio_ids;
