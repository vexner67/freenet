-- +goose Up
-- A friend request left among the followers ("Оставить в подписчиках" of
-- openvk): the follow stays, but the owner's requests leave it out.
ALTER TABLE follows
    ADD COLUMN declined boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE follows
    DROP COLUMN declined;
