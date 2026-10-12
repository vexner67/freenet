-- +goose Up
-- "Тип профиля" of openvk's privacy settings: a closed page ("Закрытый")
-- shows only the names and the photo to everyone but its owner and the
-- people the owner follows. Open by default, as there.
ALTER TABLE users
    ADD COLUMN closed boolean NOT NULL DEFAULT false;

-- +goose Down
-- post was told of the closed pages: tell it they are open again.
INSERT INTO outbox (stream, kind, payload)
SELECT 'post', 'open_wall', uuid_send(id)
FROM users
WHERE closed;

ALTER TABLE users
    DROP COLUMN closed;
