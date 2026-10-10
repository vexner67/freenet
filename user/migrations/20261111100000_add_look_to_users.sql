-- +goose Up
-- More of "Интерфейс" of the settings: the theme, whether "Заполненность
-- страницы" is shown, posts marked as adult, the view of posts and the
-- clock dates are read by (minutes east of UTC, NULL for the browser's).
ALTER TABLE users
    ADD COLUMN theme           text     NOT NULL DEFAULT 'default' CHECK (theme IN ('default', 'blue', 'dark')),
    ADD COLUMN hide_rating     boolean  NOT NULL DEFAULT false,
    ADD COLUMN nsfw            text     NOT NULL DEFAULT 'blur' CHECK (nsfw IN ('blur', 'hide', 'show')),
    ADD COLUMN post_view       text     NOT NULL DEFAULT 'old' CHECK (post_view IN ('old', 'microblog')),
    ADD COLUMN timezone_offset smallint
        CHECK (timezone_offset BETWEEN -720 AND 840 AND timezone_offset % 15 = 0);

-- +goose Down
ALTER TABLE users
    DROP COLUMN timezone_offset,
    DROP COLUMN post_view,
    DROP COLUMN nsfw,
    DROP COLUMN hide_rating,
    DROP COLUMN theme;
