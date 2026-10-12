-- +goose Up
-- openvk's "Подарки": a gift of the catalogue (its name; the catalogue is
-- in the code) someone sent to a page, with a few words and, if asked,
-- anonymously. It goes with the page it was sent to and outlives the page
-- of its sender, whose name it then loses.
CREATE TABLE gifts (
    id           uuid    PRIMARY KEY,
    recipient_id uuid    NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    sender_id    uuid    REFERENCES users (id) ON DELETE SET NULL,
    gift         text    NOT NULL CHECK (gift <> ''),
    caption      text    NOT NULL DEFAULT '' CHECK (char_length(caption) <= 250),
    anonymous    boolean NOT NULL DEFAULT false
);

-- The gifts of a page, newest first, and their count.
CREATE INDEX gifts_recipient_id_idx ON gifts (recipient_id, id DESC);
-- The gifts an erased page sent.
CREATE INDEX gifts_sender_id_idx ON gifts (sender_id) WHERE sender_id IS NOT NULL;

-- "Кому видны мои подарки" of openvk's privacy settings; everyone by
-- default, as there.
ALTER TABLE users
    ADD COLUMN gifts_read text NOT NULL DEFAULT 'everyone'
        CHECK (gifts_read IN ('everyone', 'users', 'following', 'only_me'));

-- +goose Down
ALTER TABLE users
    DROP COLUMN gifts_read;

DROP TABLE gifts;
