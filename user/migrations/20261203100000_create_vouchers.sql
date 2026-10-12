-- +goose Up
-- openvk's votes ("голоса") and rating: what vouchers gave a page. Votes
-- pay for gifts and are handed to other people; the rating adds to the
-- scale of the page. Neither is written by saving the profile.
ALTER TABLE users
    ADD COLUMN votes  bigint  NOT NULL DEFAULT 0 CHECK (votes >= 0),
    ADD COLUMN rating integer NOT NULL DEFAULT 0 CHECK (rating >= 0);

-- openvk's "Ваучеры": a serial number of 24 letters and digits that gives
-- every account entering it votes, rating or both. usages_left is how
-- many more accounts may use it, NULL for any number.
CREATE TABLE vouchers (
    id          uuid    PRIMARY KEY,
    token       text    NOT NULL UNIQUE CHECK (token ~ '^[A-Z0-9]{24}$'),
    votes       bigint  NOT NULL CHECK (votes >= 0),
    rating      integer NOT NULL CHECK (rating >= 0),
    usages_left integer CHECK (usages_left >= 0),
    CHECK (votes > 0 OR rating > 0)
);

-- Who has used a voucher: once per account.
CREATE TABLE voucher_users (
    voucher_id uuid NOT NULL REFERENCES vouchers (id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    PRIMARY KEY (voucher_id, user_id)
);

-- The vouchers of an erased page.
CREATE INDEX voucher_users_user_id_idx ON voucher_users (user_id);

-- +goose Down
DROP TABLE voucher_users;
DROP TABLE vouchers;

ALTER TABLE users
    DROP COLUMN rating,
    DROP COLUMN votes;
