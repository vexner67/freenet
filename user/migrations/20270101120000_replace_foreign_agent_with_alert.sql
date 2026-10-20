-- +goose Up
-- The orange label of openvk ("user-alert") says what the administrators
-- write, not only "иностранный агент". The pages that were marked keep the
-- marking, without the name: the site declined it, SQL cannot.
ALTER TABLE users
    ADD COLUMN alert text CHECK (char_length(alert) BETWEEN 1 AND 1000);

UPDATE users
SET alert = 'Данное сообщение (материал) создано и (или) распространено иностранным агентом либо касается деятельности иностранного агента.'
WHERE foreign_agent;

ALTER TABLE users
    DROP COLUMN foreign_agent;

-- +goose Down
ALTER TABLE users
    ADD COLUMN foreign_agent boolean NOT NULL DEFAULT false;

UPDATE users
SET foreign_agent = true
WHERE alert IS NOT NULL;

ALTER TABLE users
    DROP COLUMN alert;
