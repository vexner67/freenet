-- +goose Up
-- The card of the page a post links to: its address, title, description,
-- site and the picture media keeps as a photo of the author, as the
-- gateway read the page when the post was written. NULL for none.
ALTER TABLE posts
    ADD COLUMN link jsonb,
    ADD CONSTRAINT posts_link_check CHECK (link IS NULL OR (jsonb_typeof(link) = 'object' AND link ? 'url' AND link ? 'title'));

-- +goose Down
ALTER TABLE posts
    DROP COLUMN link;
