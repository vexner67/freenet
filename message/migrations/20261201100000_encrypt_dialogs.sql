-- +goose Up
-- Messages of dialogs are end-to-end encrypted: the browsers of the two
-- people seal what a message says, the server keeps the sealed bytes. The
-- messages written before say everything in plain text: they go, with
-- their photos and the dialogs (the owner of the site decided so).
--
-- The photos of a message and the copies of the photos of the messages it
-- carries on are deleted in media through the outbox, as erasing a page
-- does: a delete_photos record per message and per message carried on, its
-- payload the proto DeletePublishedPhotosRequest {owner_id = 1,
-- photo_ids = 2}, each a string of 36 bytes (0x0a24 and 0x1224 are the tags
-- and the length). A message has at most 10 photos, as the request takes.
-- Messages taken back have no photos left.
INSERT INTO outbox (stream, kind, payload)
SELECT 'media', 'delete_photos', p.payload
FROM (
    SELECT m.id, 0 AS place,
        '\x0a24'::bytea || convert_to(m.sender_id::text, 'UTF8') || (
            SELECT string_agg('\x1224'::bytea || convert_to(photo::text, 'UTF8'), ''::bytea ORDER BY n)
            FROM unnest(m.photo_ids) WITH ORDINALITY AS u(photo, n)
        ) AS payload
    FROM messages m
    WHERE m.deleted_at IS NULL
        AND cardinality(m.photo_ids) > 0
    UNION ALL
    SELECT m.id, f.n AS place,
        '\x0a24'::bytea || convert_to(f.carried->>'sender_id', 'UTF8') || (
            SELECT string_agg('\x1224'::bytea || convert_to(photo, 'UTF8'), ''::bytea ORDER BY k)
            FROM jsonb_array_elements_text(f.carried->'photos') WITH ORDINALITY AS u(photo, k)
        ) AS payload
    FROM messages m
    CROSS JOIN LATERAL jsonb_array_elements(m.forwarded) WITH ORDINALITY AS f(carried, n)
    WHERE m.deleted_at IS NULL
        AND jsonb_typeof(f.carried->'photos') = 'array'
        AND jsonb_array_length(f.carried->'photos') > 0
) AS p
ORDER BY p.id, p.place;

DELETE FROM messages;
DELETE FROM dialogs;

DROP INDEX messages_search_idx;

-- sealed: the version, the keys of the sender and of the recipient, the IV
-- and AES-GCM's ciphertext with its tag (message.Sealed); NULL for a
-- message that says nothing, of photos alone or taken back. text stays
-- empty: the server reads nothing of a message, the words of the messages
-- it carries on are sealed with it.
ALTER TABLE messages
    ADD COLUMN sealed bytea CHECK (octet_length(sealed) BETWEEN 93 AND 655360),
    ADD CONSTRAINT messages_plain_check CHECK (text = ''),
    DROP CONSTRAINT messages_content_check,
    ADD CONSTRAINT messages_content_check CHECK (
        deleted_at IS NOT NULL
        OR sealed IS NOT NULL
        OR cardinality(photo_ids) > 0
        OR post_id IS NOT NULL
        OR shared_photo_id IS NOT NULL
        OR jsonb_array_length(forwarded) > 0
    );

-- The public key a user's browsers seal and open messages with (X25519);
-- the private one never leaves them.
CREATE TABLE message_keys (
    user_id    uuid        PRIMARY KEY,
    public_key bytea       NOT NULL CHECK (octet_length(public_key) = 32),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
-- Sealed messages cannot be read back into plain text: they go too, with
-- their photos, as on the way up.
INSERT INTO outbox (stream, kind, payload)
SELECT 'media', 'delete_photos', p.payload
FROM (
    SELECT m.id, 0 AS place,
        '\x0a24'::bytea || convert_to(m.sender_id::text, 'UTF8') || (
            SELECT string_agg('\x1224'::bytea || convert_to(photo::text, 'UTF8'), ''::bytea ORDER BY n)
            FROM unnest(m.photo_ids) WITH ORDINALITY AS u(photo, n)
        ) AS payload
    FROM messages m
    WHERE m.deleted_at IS NULL
        AND cardinality(m.photo_ids) > 0
    UNION ALL
    SELECT m.id, f.n AS place,
        '\x0a24'::bytea || convert_to(f.carried->>'sender_id', 'UTF8') || (
            SELECT string_agg('\x1224'::bytea || convert_to(photo, 'UTF8'), ''::bytea ORDER BY k)
            FROM jsonb_array_elements_text(f.carried->'photos') WITH ORDINALITY AS u(photo, k)
        ) AS payload
    FROM messages m
    CROSS JOIN LATERAL jsonb_array_elements(m.forwarded) WITH ORDINALITY AS f(carried, n)
    WHERE m.deleted_at IS NULL
        AND jsonb_typeof(f.carried->'photos') = 'array'
        AND jsonb_array_length(f.carried->'photos') > 0
) AS p
ORDER BY p.id, p.place;

DELETE FROM messages;
DELETE FROM dialogs;

DROP TABLE message_keys;

ALTER TABLE messages
    DROP CONSTRAINT messages_content_check,
    ADD CONSTRAINT messages_content_check CHECK (
        deleted_at IS NOT NULL
        OR text <> ''
        OR cardinality(photo_ids) > 0
        OR post_id IS NOT NULL
        OR shared_photo_id IS NOT NULL
        OR jsonb_array_length(forwarded) > 0
    ),
    DROP CONSTRAINT messages_plain_check,
    DROP COLUMN sealed;

CREATE INDEX messages_search_idx ON messages USING gin (sender_id, recipient_id, message_words(text, forwarded) gin_trgm_ops);
