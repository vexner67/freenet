-- +goose Up
-- The saved settings go: at them iron paid as much as gold for a point of
-- energy. The mine is user.DefaultMine again, and the rule that a rarer ore
-- pays more now holds for whatever the administrators set next.
UPDATE mine SET settings = NULL;

-- +goose Down
-- Nothing to undo: the settings were not kept, and the old code reads NULL.
SELECT 1;
