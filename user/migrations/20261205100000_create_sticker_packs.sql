-- +goose Up
-- openvk's "Стикеры": the packs of the catalogue (in the code) each user
-- has taken from the "Магазин стикеров", free or for votes. added is
-- false once put away ("Убрать набор"): a bought pack comes back free.
CREATE TABLE sticker_packs (
    user_id  uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    pack     text        NOT NULL CHECK (pack ~ '^[a-z0-9_]{1,32}$'),
    added    boolean     NOT NULL DEFAULT true,
    taken_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, pack)
);

-- +goose Down
DROP TABLE sticker_packs;
