-- +goose Up
-- The packs people make of their photos (openvk's "Студия стикеров"): each
-- sticker is a photo of the author in media, which keeps the file. A pack
-- is named among the others by its ID in 32 hex digits, which is what
-- sticker_packs keeps for it, as for the packs of the catalogue.
CREATE TABLE custom_sticker_packs (
    id         uuid        PRIMARY KEY,
    author_id  uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name       text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 64),
    about      text        NOT NULL DEFAULT '' CHECK (char_length(about) <= 500),
    price      bigint      NOT NULL CHECK (price BETWEEN 0 AND 1000),
    published  boolean     NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX custom_sticker_packs_author_idx ON custom_sticker_packs (author_id, id);
CREATE INDEX custom_sticker_packs_published_idx ON custom_sticker_packs (id) WHERE published;

-- A photo is a sticker of one pack.
CREATE TABLE custom_stickers (
    photo_id uuid    PRIMARY KEY,
    pack_id  uuid    NOT NULL REFERENCES custom_sticker_packs (id) ON DELETE CASCADE,
    position integer NOT NULL CHECK (position BETWEEN 0 AND 39)
);

CREATE INDEX custom_stickers_pack_idx ON custom_stickers (pack_id, position);

-- Who else has taken a pack: its author may not take it away from them.
CREATE INDEX sticker_packs_pack_idx ON sticker_packs (pack);

-- +goose Down
DROP INDEX sticker_packs_pack_idx;

DELETE FROM sticker_packs WHERE pack ~ '^[0-9a-f]{32}$';

DROP TABLE custom_stickers;
DROP TABLE custom_sticker_packs;
