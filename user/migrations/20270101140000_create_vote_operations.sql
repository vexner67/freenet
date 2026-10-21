-- +goose Up
-- The statement of the bank of votes ("Голоса"): every change of the
-- votes of a page, written by the statement that makes it. amount is what
-- came in (above zero) or went out (below), balance — the votes after it;
-- peer_id is the other person of a transfer, a gift or a sticker pack,
-- note — the message of a transfer, the gift, the pack or the question of
-- an event. Rows go with their page; the peer of an erased page is
-- forgotten.
CREATE TABLE vote_operations (
    id         uuid        PRIMARY KEY,
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind       text        NOT NULL CHECK (kind IN ('transfer', 'voucher', 'gift', 'stickers', 'casino', 'aim', 'stake')),
    amount     bigint      NOT NULL CHECK (amount <> 0),
    balance    bigint      NOT NULL CHECK (balance >= 0),
    peer_id    uuid        REFERENCES users (id) ON DELETE SET NULL,
    note       text        NOT NULL DEFAULT '' CHECK (char_length(note) <= 200),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- The statement of a page, newest first.
CREATE INDEX vote_operations_user_id_idx ON vote_operations (user_id, id DESC);

-- The operations an erased page took part in as the peer.
CREATE INDEX vote_operations_peer_id_idx ON vote_operations (peer_id) WHERE peer_id IS NOT NULL;

-- +goose Down
DROP TABLE vote_operations;
