-- +goose Up
-- "Предложка" of openvk: only the admins post, others suggest posts.
ALTER TABLE clubs
    DROP CONSTRAINT clubs_wall_check,
    ADD CONSTRAINT clubs_wall_check CHECK (wall IN ('open', 'suggest', 'closed'));

-- +goose Down
-- Without suggestions only the admins post: the nearest wall is closed.
UPDATE clubs SET wall = 'closed' WHERE wall = 'suggest';

ALTER TABLE clubs
    DROP CONSTRAINT clubs_wall_check,
    ADD CONSTRAINT clubs_wall_check CHECK (wall IN ('open', 'closed'));
