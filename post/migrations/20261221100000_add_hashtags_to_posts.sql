-- +goose Up
-- The tags a post says ("#поход"), in lower case and without "#", as
-- post.Text.Hashtags finds them: the search of posts finds a tag in the
-- array, by its index, and not as a part of a longer one.
ALTER TABLE posts ADD COLUMN hashtags text[] NOT NULL DEFAULT '{}';

-- The posts written before: "#" at the start or after a space, then
-- letters, digits and "_", up to 64 of them.
UPDATE posts
SET hashtags = ARRAY(
	SELECT DISTINCT lower(m[2])
	FROM regexp_matches(text, '(^|\s)#([[:alnum:]_]+)', 'g') AS m
	WHERE char_length(m[2]) <= 64
)
WHERE text LIKE '%#%';

CREATE INDEX posts_hashtags_idx ON posts USING gin (hashtags);

-- +goose Down
DROP INDEX posts_hashtags_idx;

ALTER TABLE posts DROP COLUMN hashtags;
