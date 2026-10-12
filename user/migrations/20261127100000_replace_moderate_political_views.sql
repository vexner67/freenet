-- +goose Up
-- "Moderate" was taken off the list of political views; whoever chose it
-- has none chosen now.
UPDATE users SET political_views = '' WHERE political_views = 'moderate';

-- +goose Down
-- The views added with this change are unknown to the code before it.
UPDATE users SET political_views = '' WHERE political_views IN ('moderate_islamism', 'radical_islamism');
